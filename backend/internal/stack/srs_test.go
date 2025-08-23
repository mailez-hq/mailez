package stack

import (
	"testing"
)

func TestSRSRoundTrip(t *testing.T) {
	s := newSRSCodec("test-secret")
	srs := s.forward("john.doe", "example.org", "mail.example.com")
	if !isSRSAddress(srs) {
		t.Fatalf("expected SRS address, got %q", srs)
	}
	original, ok := s.reverse(srs)
	if !ok {
		t.Fatalf("reverse failed for %q", srs)
	}
	if original != "john.doe@example.org" {
		t.Fatalf("round trip mismatch: %q", original)
	}
}

func TestSRSRejectsTampered(t *testing.T) {
	s := newSRSCodec("test-secret")
	srs := s.forward("john", "example.org", "mail.example.com")
	// flip a character inside the body
	body := []byte(srs)
	body[12] = 'x'
	if _, ok := s.reverse(string(body)); ok {
		t.Fatal("tampered SRS address accepted")
	}
	// wrong secret must reject too
	other := newSRSCodec("other-secret")
	if _, ok := other.reverse(srs); ok {
		t.Fatal("SRS accepted with wrong secret")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(nil, 2)
	if rl.hit("a@b.c") {
		t.Fatal("hit on first increment")
	}
	if rl.hit("a@b.c") {
		t.Fatal("hit on second increment")
	}
	if !rl.hit("a@b.c") {
		t.Fatal("expected limit hit on third increment")
	}
	// different key unaffected
	if rl.hit("other@b.c") {
		t.Fatal("unrelated key should not be limited")
	}
}
