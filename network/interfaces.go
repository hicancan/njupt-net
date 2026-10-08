// Package network provides explicitly selected IPv4 links for campus requests.
package network

import (
	"fmt"
	"net"
)

type Interface struct {
	Name  string   `json:"name"`
	Index int      `json:"index"`
	Up    bool     `json:"up"`
	IPv4  []string `json:"ipv4"`
}

func SourceAddress(interfaceName, source string) (string, error) {
	if (interfaceName == "") == (source == "") {
		return "", fmt.Errorf("specify exactly one interface name or source address")
	}
	items, err := Interfaces()
	if err != nil {
		return "", err
	}
	if source != "" {
		ip := net.ParseIP(source)
		if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsLoopback() {
			return "", fmt.Errorf("source must be an assigned non-loopback IPv4 address")
		}
		for _, item := range items {
			for _, candidate := range item.IPv4 {
				if item.Up && candidate == ip.String() {
					return candidate, nil
				}
			}
		}
		return "", fmt.Errorf("source IPv4 is not assigned to an active interface")
	}
	for _, item := range items {
		if item.Name == interfaceName {
			if !item.Up {
				return "", fmt.Errorf("interface %q is down", interfaceName)
			}
			if len(item.IPv4) != 1 {
				return "", fmt.Errorf("interface %q has %d IPv4 addresses; select a source address explicitly", interfaceName, len(item.IPv4))
			}
			return item.IPv4[0], nil
		}
	}
	return "", fmt.Errorf("interface %q does not exist", interfaceName)
}
