package network

import (
	"encoding/binary"
	"syscall"
)

const socketInterfaceOption = 31 // IP_UNICAST_IF

func setSocketInterface(fd uintptr, index int) error {
	// Winsock takes the IPv4 interface index in network byte order.
	var value [4]byte
	binary.BigEndian.PutUint32(value[:], uint32(index))
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, socketInterfaceOption, int(binary.NativeEndian.Uint32(value[:])))
}
