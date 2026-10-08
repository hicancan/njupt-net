package network

import "syscall"

func getSocketInterface(fd uintptr) (int, error) {
	return syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, socketInterfaceOption)
}
