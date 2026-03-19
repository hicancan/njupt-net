package portal

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"strings"
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

func buildLogin801Payload(account, password string) []byte {
	payload, _ := json.Marshal(struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{
		Username: strings.TrimSpace(account),
		Password: md5Hex(password),
	})
	return payload
}

func buildLogout801Query(ip string) map[string]string {
	return map[string]string{
		"c":   "ACSetting",
		"a":   "Logout",
		"mip": ip,
	}
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
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
