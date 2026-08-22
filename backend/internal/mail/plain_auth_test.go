package mail

import "testing"

func TestPlainAuthStart(t *testing.T) {
	a := NewPlainAuth("amy@example.com", "token-abc")
	mechanism, resp, err := a.Start(nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if mechanism != "PLAIN" {
		t.Fatalf("mechanism: %q", mechanism)
	}
	want := "\x00amy@example.com\x00token-abc"
	if string(resp) != want {
		t.Fatalf("response: %q, want %q", resp, want)
	}
	if next, err := a.Next(nil, false); err != nil || next != nil {
		t.Fatalf("next: %v %v", next, err)
	}
}
