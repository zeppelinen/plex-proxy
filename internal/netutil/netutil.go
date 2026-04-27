package netutil

import (
	"errors"
	"net"
)

func FirstNonLoopbackIPv4() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}
			ip4 := ip.To4()
			if ip4 != nil && !ip4.IsLoopback() {
				return ip4.String(), nil
			}
		}
	}
	return "", errors.New("no non-loopback ipv4 address found")
}
