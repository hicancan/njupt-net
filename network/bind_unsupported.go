//go:build !windows && !linux && !darwin

package network

import (
	"fmt"
	"runtime"
)

func setSocketInterface(uintptr, int) error {
	return fmt.Errorf("IPv4 interface binding is unavailable on %s", runtime.GOOS)
}
