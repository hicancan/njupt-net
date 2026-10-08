package network

import "syscall"

const socketInterfaceOption = 62 // SO_BINDTOIFINDEX

func setSocketInterface(fd uintptr, index int) error {
	// TCP routes use sk_bound_dev_if, not the UDP IP_UNICAST_IF field.
	// Linux 5.7+ permits the first interface binding without CAP_NET_RAW.
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, socketInterfaceOption, index)
}
