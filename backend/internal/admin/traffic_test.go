package admin

import (
	"strings"
	"testing"
)

func TestParsePrometheus(t *testing.T) {
	body := strings.NewReader(`# HELP mailezine_smtp_messages_in_total Messages accepted.
# TYPE mailezine_smtp_messages_in_total counter
mailezine_smtp_messages_in_total{outcome="accepted"} 120
mailezine_smtp_messages_in_total{outcome="rejected"} 7
mailezine_smtp_messages_in_total{outcome="deferred"} 3
mailezine_queue_messages_total{event="delivered"} 95
mailezine_queue_messages_total{event="bounced"} 2
mailezine_queue_messages_total{event="deferred"} 1.5e+01
mailezine_queue_depth{state="pending"} 4
mailezine_queue_depth{state="delayed"} 6
mailezine_imap_sessions_active 12
bogus line without value
`)
	p, err := parsePrometheus(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.InAccepted != 120 || p.InRejected != 7 || p.InDeferred != 3 {
		t.Fatalf("in counters: %+v", p)
	}
	if p.OutDelivered != 95 || p.OutBounced != 2 || p.OutDeferred != 15 {
		t.Fatalf("out counters: %+v", p)
	}
	if p.QueueDepth != 10 {
		t.Fatalf("queue depth sum: %d", p.QueueDepth)
	}
}

func TestDeltaRestartDetection(t *testing.T) {
	if got := delta(30, 10); got != 20 {
		t.Fatalf("normal delta: %d", got)
	}
	// Engine restart resets counters: falling value counts as absolute.
	if got := delta(5, 10); got != 5 {
		t.Fatalf("restart delta: %d", got)
	}
}
