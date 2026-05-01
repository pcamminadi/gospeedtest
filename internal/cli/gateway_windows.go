//go:build windows

package cli

import (
	"os/exec"
	"strings"
)

// defaultGateway parses `route print 0.0.0.0` on Windows.
func defaultGateway() (string, error) {
	out, err := exec.Command("route", "print", "0.0.0.0").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// Active route lines start with "0.0.0.0" "<mask>" "<gateway>" ...
		if len(fields) >= 3 && fields[0] == "0.0.0.0" && fields[1] == "0.0.0.0" {
			return fields[2], nil
		}
	}
	return "", nil
}
