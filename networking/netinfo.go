package networking

// Mostly stolen from https://github.com/gokrazy

import (
	"net"
	"strings"
)

// IsInPrivateNet reports whether ip is private or not.
func IsInPrivateNet(ip net.IP) bool {
	return isPrivate("", ip)
}

func isPrivate(iface string, ip net.IP) bool {
	if strings.HasPrefix(iface, "uplink") {
		return false
	}
	switch {
	case ip.IsPrivate(),
		ip.IsLoopback(),
		ip.IsLinkLocalUnicast(),
		ip.IsMulticast():
		return true
	default:
		return false
	}

}

func interfaceAddrs(keep func(string, net.IP) bool) ([]string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var hosts []string
	for _, i := range ifaces {
		if i.Flags&net.FlagUp != net.FlagUp {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			return nil, err
		}

		for _, a := range addrs {
			ipaddr, _, err := net.ParseCIDR(a.String())
			if err != nil {
				return nil, err
			}

			if !keep(i.Name, ipaddr) {
				continue
			}

			host := ipaddr.String()
			if ipaddr.IsLinkLocalUnicast() {
				host = host + "%" + i.Name
			}
			hosts = append(hosts, host)
		}
	}
	return hosts, nil
}

// PrivateInterfaceAddrs returns all private (as per RFC1918, RFC4193,
// RFC3330, RFC3513, RFC3927, RFC4291) host addresses of all active
// interfaces, suitable to be passed to net.JoinHostPort.
func PrivateInterfaceAddrs() ([]string, error) {
	return interfaceAddrs(isPrivate)
}

// PublicInterfaceAddrs returns all public (excluding RFC1918, RFC4193,
// RFC3330, RFC3513, RFC3927, RFC4291) host addresses of all active
// interfaces, suitable to be passed to net.JoinHostPort.
func PublicInterfaceAddrs() ([]string, error) {
	return interfaceAddrs(func(iface string, addr net.IP) bool {
		return !isPrivate(iface, addr)
	})
}
