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

// Binary dhcp4d hands out DHCPv4 leases to clients.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/renameio"
	"github.com/krolaw/dhcp4"
	"github.com/krolaw/dhcp4/conn"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"git.tcp.direct/kayos/rout5/dhcp/dhcp4d"
	"git.tcp.direct/kayos/rout5/ipc"
	"git.tcp.direct/kayos/rout5/multilisten"
	"git.tcp.direct/kayos/rout5/networking"
	"git.tcp.direct/kayos/rout5/util/oui"
)

var iface = flag.String("interface", "lan0", "ethernet interface to listen for DHCPv4 requests on")

var nonExpiredLeases = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "non_expired_leases",
	Help: "Number of non-expired DHCP leases",
})

func updateNonExpired(leases []*dhcp4d.Lease) {
	now := time.Now()
	nonExpired := 0
	for _, l := range leases {
		if l.Expired(now) {
			continue
		}
		nonExpired++
	}
	nonExpiredLeases.Set(float64(nonExpired))
}

var ouiDB = oui.NewDB("/perm/dhcp4d/oui")

var (
	leasesMu sync.Mutex
	leases   []*dhcp4d.Lease
)

var (
	timefmt = func(t time.Time) string {
		return t.Format("2006-01-02 15:04")
	}
	leasesTmpl = template.Must(template.New("").Funcs(template.FuncMap{
		"timefmt": timefmt,
		"since": func(t time.Time) string {
			dur := time.Since(t)
			if dur.Hours() > 24 {
				return timefmt(t)
			}
			return dur.Truncate(1 * time.Second).String()
		},
	}).Parse(`<!DOCTYPE html>
<head>
<meta charset="utf-8">
<title>DHCPv4 status</title>
<style type="text/css">
body {
  margin-left: 1em;
}
td, th {
  padding-left: 1em;
  padding-right: 1em;
  padding-bottom: .25em;
}
td:first-child, th:first-child {
  padding-left: .25em;
}
td:last-child, th:last-child {
  padding-right: .25em;
}
th {
  padding-top: 1em;
  text-align: left;
}
span.active, span.expired, span.static, span.hostname-override {
  min-width: 5em;
  display: inline-block;
  text-align: center;
  border: 1px solid grey;
  border-radius: 5px;
}
span.active {
  background-color: #00f000;
}
span.expired {
  background-color: #f00000;
}
span.hostname-override {
  min-width: 1em;
  background-color: orange;
}
.ipaddr, .hwaddr {
  font-family: monospace;
}
tr:nth-child(even) {
  background: #eee;
}
form {
  display: inline;
}
</style>
</head>
<body>
{{ define "table" }}
<tr>
<th>IP address</th>
<th>Hostname</th>
<th>MAC address</th>
<th>Vendor</th>
<th>Expiry</th>
</tr>
{{ range $idx, $l := . }}
<tr>
<td class="ipaddr">{{$l.Addr}}</td>
<td>
<form action="/sethostname" method="post">
<input type="hidden" name="hardwareaddr" value="{{$l.HardwareAddr}}">
<input type="text" name="hostname" value="{{$l.Hostname}}">
</form>
{{ if (ne $l.HostnameOverride "") }}
<span class="hostname-override">!</span>
{{ end }}
</td>
<td class="hwaddr">{{$l.HardwareAddr}}</td>
<td>{{$l.Vendor}}</td>
<td title="{{ timefmt $l.Expiry }}">
{{ if $l.Expired }}
{{ since $l.Expiry }}
<span class="expired">expired</span>
{{ else }}
{{ if $l.Static }}
<span class="static">static</span>
{{ else }}
{{ timefmt $l.Expiry }}
<span class="active">active</span>
{{ end }}
{{ end }}
</td>
</tr>
{{ end }}
{{ end }}

<table cellpadding="0" cellspacing="0">
{{ template "table" .StaticLeases }}
{{ template "table" .DynamicLeases }}
</table>
</body>
</html>
`))
)

var httpListeners = multilisten.NewPool()

func updateListeners() error {
	hosts, err := networking.PrivateInterfaceAddrs()
	if err != nil {
		return err
	}
	if net1, err := multilisten.IPv6Net1("/perm"); err == nil {
		hosts = append(hosts, net1)
	}

	httpListeners.ListenAndServe(hosts, func(host string) multilisten.Listener {
		return &http.Server{Addr: net.JoinHostPort(host, "8067")}
	})
	return nil
}

type srv struct {
	errs   chan error
	leases func(newLeases []*dhcp4d.Lease, latest *dhcp4d.Lease)
}

func newSrv(permDir string) (*srv, error) {
	http.Handle("/metrics", promhttp.Handler())
	if err := updateListeners(); err != nil {
		return nil, err
	}
	go func() {
		ch := make(chan os.Signal, 1)
		ipc.Notify(ch, ipc.SigUSR1)
		for range ch {
			if err := updateListeners(); err != nil {
				log.Printf("updateListeners: %v", err)
			}
		}
	}()

	if err := os.MkdirAll(filepath.Join(permDir, "dhcp4d"), 0755); err != nil {
		return nil, err
	}
	errs := make(chan error)
	ifc, err := net.InterfaceByName(*iface)
	if err != nil {
		return nil, err
	}
	handler, err := dhcp4d.NewHandler(permDir, ifc, *iface, nil)
	if err != nil {
		return nil, err
	}

	http.HandleFunc("/sethostname", handleSetHostname)

	http.HandleFunc("/lease/", func(w http.ResponseWriter, r *http.Request) {
		hostname := strings.TrimPrefix(r.URL.Path, "/lease/")
		if hostname == "" {
			http.Error(w, "syntax: /lease/<hostname>", http.StatusBadRequest)
			return
		}
		leasesMu.Lock()
		defer leasesMu.Unlock()
		var lease *dhcp4d.Lease
		for _, l := range leases {
			if l.Hostname != hostname {
				continue
			}
			lease = l
			break
		}
		if lease == nil {
			http.Error(w, "no lease found", http.StatusNotFound)
			return
		}
		b, err := json.Marshal(lease)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Lease-Active", fmt.Sprint(lease.Expiry.After(time.Now().Add(handler.LeasePeriod*2/3))))
		if _, err := io.Copy(w, bytes.NewReader(b)); err != nil {
			log.Printf("/lease/%s: %v", hostname, err)
		}
	})

	http.HandleFunc("/", handleHome)

	handler.Leases = func(newLeases []*dhcp4d.Lease, latest *dhcp4d.Lease) {
		leasesMu.Lock()
		defer leasesMu.Unlock()
		leases = newLeases
		log.Printf("DHCPACK %+v", latest)
		b, err := json.Marshal(leases)
		if err != nil {
			errs <- err
			return
		}
		var out bytes.Buffer
		if err := json.Indent(&out, b, "", "\t"); err == nil {
			b = out.Bytes()
		}
		if err := renameio.WriteFile(filepath.Join(permDir, "dhcp4d/leases.json"), b, 0644); err != nil {
			errs <- err
		}
		updateNonExpired(leases)

	}
	c, err := conn.NewUDP4BoundListener(*iface, ":67")
	if err != nil {
		return nil, err
	}
	go func() {
		errs <- dhcp4.Serve(c, handler)
	}()
	return &srv{
		errs,
		handler.Leases,
	}, nil
}

func (s *srv) run(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-s.errs:
		return err
	}
}

func main() {
	// TODO: drop privileges, run as separate uid?
	flag.Parse()
	srv, err := newSrv("/perm")
	if err != nil {
		log.Fatal(err)
	}
	if err := srv.run(context.Background()); err != nil {
		log.Fatal(err)
	}
}
