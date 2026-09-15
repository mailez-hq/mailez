package admin

import (
	"testing"
	"time"

	"mailez/backend/internal/core/models"
)

var reconcileNow = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)

func item(status, detail string) CheckItem {
	return CheckItem{ID: "x", Status: status, Detail: detail}
}

func observed(pairs ...string) map[string]CheckItem {
	m := map[string]CheckItem{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = item(pairs[i+1], "detail "+pairs[i])
	}
	return m
}

func stored(pairs ...string) map[string]models.HealthSnapshot {
	m := map[string]models.HealthSnapshot{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = models.HealthSnapshot{Key: pairs[i], Status: pairs[i+1], Since: reconcileNow}
	}
	return m
}

// withPending stages a candidate status on a stored snapshot.
func withPending(m map[string]models.HealthSnapshot, key, pending string, since time.Time) map[string]models.HealthSnapshot {
	s := m[key]
	s.Pending = pending
	s.PendingSince = since
	m[key] = s
	return m
}

// First sight of a probe only records a baseline.
func TestReconcileBaseline(t *testing.T) {
	events, rows, drop := reconcileProbes(nil, observed("system:cert", "ok"), nil, reconcileNow)
	if len(events) != 0 || len(drop) != 0 {
		t.Fatalf("baseline produced events=%v drop=%v", events, drop)
	}
	if len(rows) != 1 || rows[0].Status != "ok" || rows[0].Pending != "" {
		t.Fatalf("baseline row wrong: %+v", rows)
	}
}

// A degradation must be observed twice before it is announced.
func TestReconcileFlapSuppression(t *testing.T) {
	key := "domain:example.com:mx"
	_, rows, _ := reconcileProbes(stored(key, "ok"), observed(key, "fail"), nil, reconcileNow)
	if len(rows) != 1 || rows[0].Pending != "fail" {
		t.Fatalf("first fail run should stage pending: %+v", rows)
	}
	events, rows, _ := reconcileProbes(
		map[string]models.HealthSnapshot{key: rows[0]},
		observed(key, "fail"), nil, reconcileNow)
	if len(events) != 1 || events[0].From != "ok" || events[0].To != "fail" {
		t.Fatalf("second fail run should announce: %+v", events)
	}
	if rows[0].Status != "fail" || rows[0].Pending != "" {
		t.Fatalf("announced status not persisted: %+v", rows[0])
	}
	if events[0].Detail != "detail "+key {
		t.Fatalf("event detail not carried: %+v", events[0])
	}
}

// A single failed run that settles back announces nothing.
func TestReconcileSettledBack(t *testing.T) {
	key := "system:redis"
	snap := withPending(stored(key, "ok"), key, "fail", reconcileNow.Add(-time.Hour))
	events, rows, _ := reconcileProbes(snap, observed(key, "ok"), nil, reconcileNow)
	if len(events) != 0 {
		t.Fatalf("settled-back run announced: %+v", events)
	}
	if rows[0].Pending != "" {
		t.Fatalf("pending not cleared: %+v", rows[0])
	}
}

// Recovery to ok announces like any other transition.
func TestReconcileRecovery(t *testing.T) {
	key := "system:cert"
	snap := withPending(stored(key, "fail"), key, "ok", reconcileNow.Add(-time.Hour))
	events, _, _ := reconcileProbes(snap, observed(key, "ok"), nil, reconcileNow)
	if len(events) != 1 || events[0].From != "fail" || events[0].To != "ok" {
		t.Fatalf("recovery not announced: %+v", events)
	}
	if events[0].Since != snap[key].PendingSince.Format(time.RFC3339) {
		t.Fatalf("recovery since wrong: %+v", events[0])
	}
}

// Probes that vanish are dropped from memory.
func TestReconcileDropVanished(t *testing.T) {
	key := "domain:old.example:mx"
	_, rows, drop := reconcileProbes(stored(key, "ok"), nil, nil, reconcileNow)
	if len(rows) != 0 || len(drop) != 1 || drop[0] != key {
		t.Fatalf("vanished probe: rows=%v drop=%v", rows, drop)
	}
}

// Muted probes are neither diffed nor dropped.
func TestReconcileMuted(t *testing.T) {
	key := "system:cert"
	events, rows, drop := reconcileProbes(stored(key, "ok"), observed(key, "fail"), mutedKeys(key+", bad , "), reconcileNow)
	if len(events) != 0 || len(rows) != 0 || len(drop) != 0 {
		t.Fatalf("muted probe touched: events=%v rows=%v drop=%v", events, rows, drop)
	}
}

// A changed candidate replaces the staged one: the newest observation is
// what must repeat.
func TestReconcileCandidateReplaced(t *testing.T) {
	key := "domain:example.com:mx"
	snap := withPending(stored(key, "ok"), key, "warn", reconcileNow.Add(-time.Hour))
	_, rows, _ := reconcileProbes(snap, observed(key, "fail"), nil, reconcileNow)
	if rows[0].Pending != "fail" {
		t.Fatalf("candidate not replaced: %+v", rows[0])
	}
}

func TestProbeLabel(t *testing.T) {
	cases := []struct{ key, want string }{
		{"domain:example.com:mx", "example.com · mx"},
		{"system:cert", "系统 · cert"},
		{"bare", "bare"},
	}
	for _, c := range cases {
		if got := probeLabel(c.key); got != c.want {
			t.Errorf("probeLabel(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}
