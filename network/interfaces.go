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

func Interfaces() ([]Interface, error) {
	items, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]Interface, 0, len(items))
	for _, item := range items {
		entry := Interface{Name: item.Name, Index: item.Index, Up: item.Flags&net.FlagUp != 0, IPv4: []string{}}
		addresses, err := item.Addrs()
		if err != nil {
			return nil, fmt.Errorf("addresses for %q: %w", item.Name, err)
		}
		for _, address := range addresses {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && ip.To4() != nil {
				entry.IPv4 = append(entry.IPv4, ip.String())
			}
		}
		result = append(result, entry)
	}
	return result, nil
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
