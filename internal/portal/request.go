package portal

import (
	"fmt"
	"strings"
	"time"
)

func buildLogin802Query(account, password, ip, isp string) map[string]string {
	return map[string]string{
		"callback":      jsonpCallback,
		"login_method":  "1",
		"user_account":  ",0," + strings.TrimSpace(account) + ispSuffix(isp),
		"user_password": password,
		"wlan_user_ip":  ip,
	}
}

func buildLogout802Query(ip string) map[string]string {
	return map[string]string{
		"callback":     jsonpCallback,
		"login_method": "1",
		"wlan_user_ip": ip,
	}
}

func buildLogin801Query(account, password, ip, ipv6 string) map[string]string {
	return map[string]string{
		"c":     "ACSetting",
		"a":     "Login",
		"DDDDD": account,
		"upass": password,
		"mip":   ip,
		"v6ip":  ipv6,
		"timet": fmt.Sprintf("%d", time.Now().Unix()),
	}
}

func buildLogout801Query(ip string) map[string]string {
	return map[string]string{
		"c":   "ACSetting",
		"a":   "Logout",
		"mip": ip,
	}
}

func ispSuffix(isp string) string {
	switch strings.ToLower(strings.TrimSpace(isp)) {
	case "telecom":
		return "@dx"
	case "unicom":
		return "@lt"
	case "mobile":
		return "@cmcc"
	default:
		return ""
	}
}
