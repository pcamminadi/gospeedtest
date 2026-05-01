//go:build darwin || linux || freebsd || openbsd || netbsd

package cli

import (
	"errors"
	"os/exec"
	"runtime"
	"strings"
)

// defaultGateway returns the IP of the default route via the system's
// `route` / `ip` command. Best-effort; an empty string is returned on any
// error and the UI shows a "—".
func defaultGateway() (string, error) {
	switch runtime.GOOS {
	case "darwin", "freebsd", "openbsd", "netbsd":
		out, err := exec.Command("route", "-n", "get", "default").Output()
		if err != nil {
			return "", err
		}
		return parseRouteOutput(string(out)), nil
	case "linux":
		// Try `ip route` first, fall back to `route -n`.
		if out, err := exec.Command("ip", "route", "show", "default").Output(); err == nil {
			return parseIPRouteOutput(string(out)), nil
		}
		out, err := exec.Command("route", "-n").Output()
		if err != nil {
			return "", err
		}
		return parseLinuxRouteOutput(string(out)), nil
	}
	return "", errors.New("unsupported os")
}

// parseRouteOutput pulls the gateway from BSD-style `route -n get default`.
//
//	   route to: default
//	destination: default
//	    gateway: 192.168.1.1
//	    interface: en0
func parseRouteOutput(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gateway:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "gateway:"))
		}
	}
	return ""
}

// parseIPRouteOutput pulls the gateway from `ip route show default`.
//
//	default via 192.168.1.1 dev wlan0 proto dhcp ...
func parseIPRouteOutput(s string) string {
	fields := strings.Fields(s)
	for i, f := range fields {
		if f == "via" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// parseLinuxRouteOutput pulls the gateway from `route -n` (legacy net-tools).
func parseLinuxRouteOutput(s string) string {
	for _, line := range strings.Split(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "0.0.0.0" {
			return fields[1]
		}
	}
	return ""
}
