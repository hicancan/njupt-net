package network

import (
	"fmt"
	"syscall"
)

func sourceInterface(items []Interface, source string) (int, error) {
	index := 0
	for _, item := range items {
		if !item.Up {
			continue
		}
		for _, address := range item.IPv4 {
			if address != source {
				continue
			}
			if item.Index <= 0 {
				return 0, fmt.Errorf("source IPv4 has an invalid interface index")
			}
			if index != 0 && index != item.Index {
				return 0, fmt.Errorf("source IPv4 is assigned to multiple active interfaces")
			}
			index = item.Index
		}
	}
	if index == 0 {
		return 0, fmt.Errorf("source IPv4 is not assigned to an active interface")
	}
	return index, nil
}

// interfaceControl runs before bind/connect so route selection uses this device.
func interfaceControl(index int) func(string, string, syscall.RawConn) error {
	return func(_, _ string, connection syscall.RawConn) error {
		var optionErr error
		if err := connection.Control(func(fd uintptr) {
			optionErr = setSocketInterface(fd, index)
		}); err != nil {
			return fmt.Errorf("control IPv4 socket: %w", err)
		}
		if optionErr != nil {
			return fmt.Errorf("bind IPv4 socket to interface %d: %w", index, optionErr)
		}
		return nil
	}
}
