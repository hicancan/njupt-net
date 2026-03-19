//go:build !windows

package guard

import "syscall"

func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func defaultProcessExists(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func defaultFindLegacyPIDs() ([]int, error) {
	return nil, nil
}
