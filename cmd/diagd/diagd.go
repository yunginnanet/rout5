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

// Binary diagd provides automated network diagnostics.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"sync"

	diag2 "git.tcp.direct/kayos/rout5/diag"
	"git.tcp.direct/kayos/rout5/multilisten"
)

var httpListeners = multilisten.NewPool()

func updateListeners() error {
	hosts, err := networking.PrivateInterfaceAddrs()
	if err != nil {
		return err
	}

	httpListeners.ListenAndServe(hosts, func(host string) multilisten.Listener {
		return &http.Server{Addr: net.JoinHostPort(host, "7733")}
	})
	return nil
}

func dump(indent int, w io.Writer, re *diag2.EvalResult) {
	symbol := "✔"
	if re.Error {
		symbol = "✘"
	}
	fmt.Fprintf(w, "<li>\n%s%s %s: %s<ul>",
		strings.Repeat("  ", indent),
		symbol,
		html.EscapeString(re.Name),
		html.EscapeString(re.Status))
	for _, ch := range re.Children {
		dump(indent+1, w, ch)
	}
	fmt.Fprintf(w, "</ul></li>")
}

func firstError(re *diag2.EvalResult) string {
	if re.Error {
		return fmt.Sprintf("%s: %s", re.Name, re.Status)
	}
	for _, ch := range re.Children {
		if msg := firstError(ch); msg != "" {
			return msg
		}
	}
	return ""
}

func logic() error {
	var (
		ifname = flag.String("interface",
			"uplink0",
			"interface name to query")
	)
	const (
		ip6allrouters = "ff02::2" // no /etc/hosts on gokrazy
	)
	flag.Parse()
	uplink := *ifname
	m := diag2.NewMonitor(diag2.Link(uplink).
		Then(diag2.DHCPv4().
			Then(diag2.Ping4Gateway().
				Then(diag2.Ping4("google.ch").
					Then(diag2.TCP4("www.google.ch:80"))))).
		Then(diag2.DHCPv6().
			Then(diag2.Ping6("lan0", "google.ch"))).
		Then(diag2.RouterAdvertisments(uplink).
			Then(diag2.Ping6Gateway().
				Then(diag2.Ping6(uplink, "google.ch").
					Then(diag2.TCP6("www.google.ch:80"))))).
		Then(diag2.Ping6("", ip6allrouters+"%"+uplink)))
	var mu sync.Mutex
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		re := m.Evaluate()
		mu.Unlock()
		fmt.Fprintf(w, `<!DOCTYPE html><style type="text/css">ul { list-style-type: none; }</style><ul>`)
		dump(0, w, re)
	})
	http.HandleFunc("/health.json", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		re := m.Evaluate()
		mu.Unlock()
		reply := struct {
			FirstError string `json:"first_error"`
		}{
			FirstError: firstError(re),
		}
		b, err := json.Marshal(&reply)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Write(b)
	})
	if err := updateListeners(); err != nil {
		return err
	}
	ch := make(chan os.Signal, 1)
	ipc.Notify(ch, ipc.SigUSR1)
	for range ch {
		if err := updateListeners(); err != nil {
			log.Printf("updateListeners: %v", err)
		}
	}
	return nil
}

func main() {
	if err := logic(); err != nil {
		log.Fatal(err)
	}
}
