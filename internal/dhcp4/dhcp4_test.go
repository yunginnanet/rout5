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

package dhcp4

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"git.tcp.direct/kayos/rout5/internal/testing/pcapreplayer"
)

func TestDHCP4(t *testing.T) {
	pcappath := os.Getenv("ROUTER7_PCAP_DIR")
	if pcappath != "" {
		pcappath = filepath.Join(pcappath, "dhcp4.pcap")
	}
	conn, err := pcapreplayer.NewDHCP4Conn("testdata/fiber7.pcap", pcappath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	mac, err := net.ParseMAC("d8:58:d7:00:4e:df")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	c := Client{
		hardwareAddr: mac,
		timeNow:      func() time.Time { return now },
		connection:   conn,
		generateXID: func() uint32 {
			// TODO: read the transaction ID from the pcap file
			return 0x7708d724
		},
	}

	c.ObtainOrRenew()
	if err := c.Err(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := c.Config()
	want := Config{
		RenewAfter: now.Add(13*time.Minute + 24*time.Second),
		ClientIP:   "85.195.207.62",
		SubnetMask: "255.255.255.128",
		Router:     "85.195.207.1",
		DNS: []string{
			"77.109.128.2",
			"213.144.129.20",
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected config: diff (-want +got):\n%s", diff)
	}
}
