package network

import "syscall"

const socketInterfaceOption = 25 // IP_BOUND_IF

func setSocketInterface(fd uintptr, index int) error {
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, socketInterfaceOption, index)
}
