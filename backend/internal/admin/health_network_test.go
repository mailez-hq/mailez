package admin

import (
	"reflect"
	"testing"
)

func TestParseResolvConf(t *testing.T) {
	cases := []struct {
		name   string
		data   string
		ns     []string
		public []string
	}{
		{
			"mixed private and public",
			"# generated\nnameserver 127.0.0.11\nnameserver 8.8.8.8\noptions edns0\n",
			[]string{"127.0.0.11", "8.8.8.8"},
			[]string{"8.8.8.8"},
		},
		{
			"public ipv6",
			"nameserver 2606:4700:4700::1111\n",
			[]string{"2606:4700:4700::1111"},
			[]string{"2606:4700:4700::1111"},
		},
		{
			"docker embedded dns only",
			"nameserver 127.0.0.11\n",
			[]string{"127.0.0.11"},
			nil,
		},
		{
			"no nameservers",
			"search example.com\n",
			nil, nil,
		},
		{
			"empty",
			"",
			nil, nil,
		},
	}
	for _, c := range cases {
		ns, public := parseResolvConf(c.data)
		if !reflect.DeepEqual(ns, c.ns) || !reflect.DeepEqual(public, c.public) {
			t.Errorf("%s: parseResolvConf ns=%v public=%v, want ns=%v public=%v",
				c.name, ns, public, c.ns, c.public)
		}
	}
}

// An unresolvable probe host grades unknown: the port is not known to be
// blocked, the network simply cannot answer the probe.
func TestCheckOutbound25UnknownOnBadHost(t *testing.T) {
	item := checkOutbound25("probe-host.invalid.")
	if item.Status != statusUnknown {
		t.Errorf("checkOutbound25(invalid host) status = %q, want %q", item.Status, statusUnknown)
	}
}
