//go:build darwin || linux || freebsd || openbsd || netbsd

package cli

import "testing"

func TestParseRouteOutput(t *testing.T) {
	in := `   route to: default
destination: default
       mask: default
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC,PRCLONING,GLOBAL>
`
	if got := parseRouteOutput(in); got != "192.168.1.1" {
		t.Errorf("parseRouteOutput = %q, want 192.168.1.1", got)
	}
}

func TestParseRouteOutputMissingGateway(t *testing.T) {
	in := "route to: default\ndestination: default\n"
	if got := parseRouteOutput(in); got != "" {
		t.Errorf("parseRouteOutput = %q, want empty string", got)
	}
}

func TestParseIPRouteOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "wlan0",
			in:   "default via 192.168.1.1 dev wlan0 proto dhcp src 192.168.1.42 metric 600\n",
			want: "192.168.1.1",
		},
		{
			name: "ipv6",
			in:   "default via fe80::1 dev eth0 proto ra metric 1024\n",
			want: "fe80::1",
		},
		{
			name: "no via",
			in:   "default dev tun0 scope link\n",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseIPRouteOutput(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseLinuxRouteOutput(t *testing.T) {
	in := `Kernel IP routing table
Destination     Gateway         Genmask         Flags Metric Ref    Use Iface
0.0.0.0         10.0.0.1        0.0.0.0         UG    100    0        0 eth0
10.0.0.0        0.0.0.0         255.255.255.0   U     100    0        0 eth0
`
	if got := parseLinuxRouteOutput(in); got != "10.0.0.1" {
		t.Errorf("parseLinuxRouteOutput = %q, want 10.0.0.1", got)
	}
}

func TestParseLinuxRouteOutputNoDefault(t *testing.T) {
	in := `Kernel IP routing table
Destination     Gateway         Genmask         Flags Metric Ref    Use Iface
10.0.0.0        0.0.0.0         255.255.255.0   U     100    0        0 eth0
`
	if got := parseLinuxRouteOutput(in); got != "" {
		t.Errorf("parseLinuxRouteOutput = %q, want empty", got)
	}
}
