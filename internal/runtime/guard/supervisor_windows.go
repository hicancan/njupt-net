//go:build windows

package guard

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | 0x00000200,
	}
}

func defaultProcessExists(pid int) bool {
	result, err := exec.Command("tasklist", "/FI", "PID eq "+strconv.Itoa(pid), "/FO", "CSV", "/NH").CombinedOutput()
	if err != nil {
		return false
	}
	text := strings.TrimSpace(string(result))
	return text != "" && !strings.Contains(strings.ToLower(text), "no tasks are running")
}

func defaultFindLegacyPIDs() ([]int, error) {
	script := "$patterns = 'njupt_w_guard.py','run-w-guard.ps1','start-w-guard.ps1'; " +
		"Get-CimInstance Win32_Process | Where-Object { $cmd = $_.CommandLine; $cmd -and ($patterns | Where-Object { $cmd -like ('*' + $_ + '*') }) } | " +
		"Select-Object -ExpandProperty ProcessId"
	output, err := exec.Command("powershell.exe", "-NoProfile", "-Command", script).Output()
	if err != nil {
		return nil, nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	pids := []int{}
	for _, line := range lines {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		pid, err := strconv.Atoi(value)
		if err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
