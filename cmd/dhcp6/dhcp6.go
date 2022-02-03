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

// Binary dhcp6 obtains a DHCPv6 lease, persists it to
// /perm/dhcp6/wire/lease.json and notifies netconfigd.
package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/renameio"
	"github.com/jpillora/backoff"

	"git.tcp.direct/kayos/rout5/notify"

	"git.tcp.direct/kayos/rout5/internal/dhcp6"
)

func logic() error {
	const leasePath = "/perm/dhcp6/wire/lease.json"
	if err := os.MkdirAll(filepath.Dir(leasePath), 0755); err != nil {
		return err
	}

	duid, err := ioutil.ReadFile("/perm/dhcp6/duid")
	if err != nil {
		log.Printf("could not read /perm/dhcp6/duid (%v), proceeding with DUID-LLT", err)
	}

	c, err := dhcp6.NewClient(dhcp6.ClientConfig{
		InterfaceName: "uplink0",
		DUID:          duid,
	})
	if err != nil {
		return err
	}
	usr2 := make(chan os.Signal, 1)
	ipc.Notify(usr2, ipc.SigUSR2)
	boff := backoff.Backoff{
		Factor: 2,
		Jitter: true,
		Min:    10 * time.Second,
		Max:    1 * time.Minute,
	}

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
			return err
		}
		if err := notify.Process("/user/netconfigd", ipc.SigUSR1); err != nil {
			log.Printf("notifying netconfig: %v", err)
		}
		if err := notify.Process("/user/radvd", ipc.SigUSR1); err != nil {
			log.Printf("notifying radvd: %v", err)
		}
		select {
		case <-time.After(time.Until(c.Config().RenewAfter)):
			// fallthrough and renew the DHCP lease
		case <-usr2:
			log.Printf("SIGUSR2 received, sending DHCPRELEASE")
			if _, _, err := c.Release(); err != nil {
				return err
			}
			os.Exit(125) // quit supervision by gokrazy
		}
	}
	return c.Err() // permanent error
}

func main() {
	flag.Parse()
	if err := logic(); err != nil {
		log.Fatal(err)
	}
}
