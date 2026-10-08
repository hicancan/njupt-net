//go:build !windows

package network

import (
	"fmt"
	"net"
)

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
