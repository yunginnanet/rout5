// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Binary dhcp4 obtains a DHCPv4 lease, persists it to
// /perm/dhcp4/wire/lease.json and notifies netconfigd.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/renameio"
	"github.com/jpillora/backoff"
	"github.com/rs/zerolog"

	"git.tcp.direct/kayos/rout5/config"
	"git.tcp.direct/kayos/rout5/db"
	"git.tcp.direct/kayos/rout5/dhcp/dhcp4"
	"git.tcp.direct/kayos/rout5/ipc"
	"git.tcp.direct/kayos/rout5/logging"
	"git.tcp.direct/kayos/rout5/netconfig"
)

var log *zerolog.Logger

func init() {
	log = logging.GetLogger()
}

func healthy() error {
	req, err := http.NewRequest("GET", "http://localhost:7733/health.json", nil)
	if err != nil {
		return err
	}
	ctx, canc := context.WithTimeout(context.Background(), 30*time.Second)
	defer canc()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if got, want := resp.StatusCode, http.StatusOK; got != want {
		b, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("%v: got HTTP %v (%s), want HTTP status %v",
			req.URL.String(),
			resp.Status,
			string(b),
			want)
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var reply struct {
		FirstError string `json:"first_error"`
	}
	if err := json.Unmarshal(b, &reply); err != nil {
		return err
	}

	if reply.FirstError != "" {
		return errors.New(reply.FirstError)
	}

	return nil
}

func logic() error {
	// TODO: support multiple interfaces
	var netInterface = config.DHCPInterfaces[0]

	iface, err := net.InterfaceByName(netInterface)
	if err != nil {
		return err
	}
	hwaddr := iface.HardwareAddr
	// The interface may not have been configured by netconfigd yet and might
	// still use the old hardware address. We overwrite it with the address that
	// netconfigd is going to use to fix this issue without additional
	// synchronization.
	details, err := netconfig.Interface("/perm", netInterface)
	if err == nil {
		if spoof := details.SpoofHardwareAddr; spoof != "" {
			if addr, err := net.ParseMAC(spoof); err == nil {
				hwaddr = addr
			}
		}
	}
	var ack *layers.DHCPv4
	ackB, dErr := db.DHCPMessages()
	if dErr != nil {
		log.Warn().Msgf("Loading previous DHCPACK packet from database: %v", dErr)
	} else {
		pkt := gopacket.NewPacket(ackB, layers.LayerTypeDHCPv4, gopacket.DecodeOptions{})
		if dhcp, ok := pkt.Layer(layers.LayerTypeDHCPv4).(*layers.DHCPv4); ok {
			ack = dhcp
		}
	}
	c := dhcp4.Client{
		Interface: iface,
		HWAddr:    hwaddr,
		Ack:       ack,
	}
	usr2 := make(chan os.Signal, 1)
	ipc.Notify(usr2, ipc.SigUSR2)
	boff := backoff.Backoff{
		Factor: 2,
		Jitter: true,
		Min:    10 * time.Second,
		Max:    1 * time.Minute,
	}
ObtainOrRenew:
	for c.ObtainOrRenew() {
		if err := c.Err(); err != nil {
			dur := boff.Duration()
			log.Printf("Temporary error: %v (waiting %v)", err, dur)
			time.Sleep(dur)
			continue
		}
		boff.Reset()
		log.Printf("lease: %+v", c.Config())
		b, err := json.Marshal(c.Config())
		if err != nil {
			return err
		}
		if err := renameio.WriteFile(leasePath, b, 0644); err != nil {
			return fmt.Errorf("persisting lease to %s: %v", leasePath, err)
		}
		buf := gopacket.NewSerializeBuffer()
		gopacket.SerializeLayers(buf,
			gopacket.SerializeOptions{
				FixLengths:       true,
				ComputeChecksums: true,
			},
			c.Ack,
		)
		if err := renameio.WriteFile(ackFn, buf.Bytes(), 0644); err != nil {
			return fmt.Errorf("persisting DHCPACK to %s: %v", ackFn, err)
		}
		if err := ipc.Process("/user/netconfigd", ipc.SigUSR1); err != nil {
			log.Printf("notifying netconfig: %v", err)
		}

		unhealthyCycles := 0
		for {
			select {
			case <-time.After(time.Until(c.Config().RenewAfter)):
				// fallthrough and renew the DHCP lease
				continue ObtainOrRenew

			case <-time.After(1 * time.Minute):
				if err := healthy(); err == nil {
					unhealthyCycles = 0
					continue // wait another minute
				} else {
					unhealthyCycles++
					log.Printf("router unhealthy (cycle %d of 5): %v", unhealthyCycles, err)
					if unhealthyCycles < 20 {
						continue // wait until unhealthy for longer
					}
					// fallthrough
				}
				// Still not healthy? Drop DHCP lease and start from scratch.
				log.Printf("unhealthy for 5 cycles, starting over without lease")
				c.Ack = nil

			case <-usr2:
				log.Printf("SIGUSR2 received, sending DHCPRELEASE")
				if err := c.Release(); err != nil {
					return err
				}
				// Ensure dhcp4 does start from scratch next time
				// by deleting the DHCPACK file:
				if err := os.Remove(ackFn); err != nil && !os.IsNotExist(err) {
					return err
				}
				os.Exit(125) // quit supervision by gokrazy
			}
		}
	}
	return c.Err() // permanent error
}

func main() {
	// TODO: drop privileges, run as separate uid?
	flag.Parse()
	if err := logic(); err != nil {
		log.Fatal(err)
	}
}
