package admin

import "testing"

// Error codes in 127.255.255.x are Spamhaus probe failures (public-resolver
// refusal, zone typo, rate limit) and must grade unknown, never "listed".
func TestOnlySpamhausErrCode(t *testing.T) {
	cases := []struct {
		name string
		ans  []string
		want bool
	}{
		{"open-resolver refusal", []string{"127.255.255.254"}, true},
		{"zone typo", []string{"127.255.255.252"}, true},
		{"rate limited", []string{"127.255.255.253"}, true},
		{"rate limited 255", []string{"127.255.255.255"}, true},
		{"mixed error codes", []string{"127.255.255.254", "127.255.255.253"}, true},
		{"sbl listing", []string{"127.0.0.2"}, false},
		{"xbl listing", []string{"127.0.0.4"}, false},
		{"pbl advisory", []string{"127.0.0.10"}, false},
		{"error mixed with listing", []string{"127.255.255.254", "127.0.0.2"}, false},
		{"empty", nil, false},
	}
	for _, c := range cases {
		if got := onlySpamhausErrCode(c.ans); got != c.want {
			t.Errorf("%s: onlySpamhausErrCode(%v) = %v, want %v", c.name, c.ans, got, c.want)
		}
	}
}

func TestOnlyPBL(t *testing.T) {
	cases := []struct {
		name string
		ans  []string
		want bool
	}{
		{"spamhaus-maintained", []string{"127.0.0.10"}, true},
		{"isp-maintained", []string{"127.0.0.11"}, true},
		{"both", []string{"127.0.0.10", "127.0.0.11"}, true},
		{"sbl is not pbl", []string{"127.0.0.2"}, false},
		{"open-resolver code is not pbl", []string{"127.255.255.254"}, false},
		{"pbl mixed with sbl", []string{"127.0.0.10", "127.0.0.2"}, false},
	}
	for _, c := range cases {
		if got := onlyPBL(c.ans); got != c.want {
			t.Errorf("%s: onlyPBL(%v) = %v, want %v", c.name, c.ans, got, c.want)
		}
	}
}
