package network

import "syscall"

func getSocketInterface(fd uintptr) (int, error) {
	// Winsock returns IP_UNICAST_IF in host byte order.
	return syscall.GetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IP, socketInterfaceOption)
}
