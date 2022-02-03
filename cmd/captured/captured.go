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

// Binary captured streams network packets to wireshark via SSH, replaying
// buffered packets upon connection for retroactive debugging.
package main

import (
	"container/ring"
	"context"
	"flag"
	"fmt"
	"log"
	_ "net/http/pprof"
	"sync"

	"github.com/gokrazy/gokrazy"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"

	"git.tcp.direct/kayos/rout5/ipc"
	"git.tcp.direct/kayos/rout5/multilisten"
)

var (
	hostKeyPath = flag.String("host_key",
		"/perm/breakglass.host_key",
		"path to a PEM-encoded RSA, DSA or ECDSA private key (create using e.g. ssh-keygen -f /perm/breakglass.host_key -N '' -t rsa)")
)

func capturePackets(ctx context.Context) (chan gopacket.Packet, error) {
	packets := make(chan gopacket.Packet)
	for _, ifname := range []string{"uplink0", "lan0"} {
		handle, err := pcapgo.NewEthernetHandle(ifname)
		if err != nil {
			return nil, fmt.Errorf("pcapgo.NewEthernetHandle(%v): %v", ifname, err)
		}

		if err := handle.SetBPF(instructions); err != nil {
			return nil, fmt.Errorf("SetBPF: %v", err)
		}

		pkgsrc := gopacket.NewPacketSource(handle, layers.LayerTypeEthernet)
		go func() {
			defer handle.Close()
			for {
				packet, err := pkgsrc.NextPacket()
				if err != nil {
					log.Printf("NextPacket: %v", err)
					return
				}
				select {
				case packets <- packet:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	return packets, nil
}

type packetRingBuffer struct {
	sync.Mutex
	r *ring.Ring
}

func newPacketRingBuffer(size int) *packetRingBuffer {
	return &packetRingBuffer{
		r: ring.New(size),
	}
}

func (prb *packetRingBuffer) writePacket(p gopacket.Packet) {
	prb.Lock()
	defer prb.Unlock()
	prb.r.Value = p
	prb.r = prb.r.Next()
}

func (prb *packetRingBuffer) packetsLocked() []gopacket.Packet {
	packets := make([]gopacket.Packet, 0, prb.r.Len())
	prb.r.Do(func(x interface{}) {
		if x != nil {
			packets = append(packets, x.(gopacket.Packet))
		}
	})
	return packets
}

var sshListeners = multilisten.NewPool()

func updateListeners(srv *server) error {
	hosts, err := gokrazy.PrivateInterfaceAddrs()
	if err != nil {
		return err
	}

	sshListeners.ListenAndServe(hosts, func(host string) multilisten.Listener {
		return srv.listenerFor(host)
	})
	return nil
}

func logic() error {
	prb := newPacketRingBuffer(50000)
	srv, err := newServer(prb)
	if err != nil {
		return err
	}
	if err := updateListeners(srv); err != nil {
		return err
	}

	go func() {
		ch := make(chan ipc.Signal, 1)
		ipc.Notify(ch, ipc.SigUSR1)
		for range ch {
			if err := updateListeners(srv); err != nil {
				log.Printf("updateListeners: %v", err)
			}
		}
	}()

	packets, err := capturePackets(context.Background())
	if err != nil {
		return err
	}
	for packet := range packets {
		prb.writePacket(packet)
	}
	return nil
}

func main() {
	flag.Parse()
	if err := logic(); err != nil {
		log.Fatal(err)
	}
}
