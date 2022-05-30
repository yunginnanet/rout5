package main

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"time"

	"git.tcp.direct/kayos/rout5/dhcp/dhcp4d"
	"git.tcp.direct/kayos/rout5/networking"
)

func handleHome(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	ip := net.ParseIP(host)
	if xff := r.Header.Get("X-Forwarded-For"); ip.IsLoopback() && xff != "" {
		ip = net.ParseIP(xff)
	}
	if !networking.IsInPrivateNet(ip) {
		http.Error(w, fmt.Sprintf("access from %v forbidden", ip), http.StatusForbidden)
		return
	}

	type tmplLease struct {
		dhcp4d.Lease

		Vendor  string
		Expired bool
		Static  bool
	}

	leasesMu.Lock()
	defer leasesMu.Unlock()
	static := make([]tmplLease, 0, len(leases))
	dynamic := make([]tmplLease, 0, len(leases))
	tl := func(l *dhcp4d.Lease) tmplLease {
		return tmplLease{
			Lease:   *l,
			Vendor:  ouiDB.Lookup(l.HardwareAddr[:8]),
			Expired: l.Expired(time.Now()),
			Static:  l.Expiry.IsZero(),
		}
	}
	for _, l := range leases {
		if l.Expiry.IsZero() {
			static = append(static, tl(l))
		} else {
			dynamic = append(dynamic, tl(l))
		}
	}
	sort.Slice(static, func(i, j int) bool {
		return static[i].Num < static[j].Num
	})
	sort.Slice(dynamic, func(i, j int) bool {
		return !dynamic[i].Expiry.Before(dynamic[j].Expiry)
	})

	if err := leasesTmpl.Execute(w, struct {
		StaticLeases  []tmplLease
		DynamicLeases []tmplLease
	}{
		StaticLeases:  static,
		DynamicLeases: dynamic,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func handleSetHostname(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "want POST", http.StatusMethodNotAllowed)
		return
	}
	hwaddr := r.FormValue("hardwareaddr")
	if hwaddr == "" {
		http.Error(w, "missing hardwareaddr parameter", http.StatusBadRequest)
		return
	}
	hostname := r.FormValue("hostname")
	if hostname == "" {
		http.Error(w, "missing hostname parameter", http.StatusBadRequest)
		return
	}
	if err := handler.SetHostname(hwaddr, hostname); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}
