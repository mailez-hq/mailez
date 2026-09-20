package push

import "testing"

// Regression: a message that landed between the subscription and the
// notifier's first read was folded into the baseline and never pushed.
func TestNotifyDeltaFirstObservation(t *testing.T) {
	cases := []struct {
		name      string
		total     int
		delivered int
		wantCount int
		wantRaise bool
	}{
		{"receipt for the first unseen mail", 1, 1, 1, true},
		{"receipt while a backlog exists", 6, 1, 1, true},
		{"receipt before the delivery is visible", 0, 1, 1, true},
		{"poll with no receipt stays silent", 4, 0, 0, false},
		{"receipt the user already read still announces", 1, 1, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, raise := notifyDelta(0, false, tc.total, tc.delivered)
			if raise != tc.wantRaise || got != tc.wantCount {
				t.Fatalf("notifyDelta(0, false, %d, %d) = (%d, %v), want (%d, %v)",
					tc.total, tc.delivered, got, raise, tc.wantCount, tc.wantRaise)
			}
		})
	}
}

func TestNotifyDeltaAgainstBaseline(t *testing.T) {
	cases := []struct {
		name      string
		prev      int
		total     int
		delivered int
		wantCount int
		wantRaise bool
	}{
		{"growth is announced", 2, 5, 0, 3, true},
		{"a receipt larger than the growth wins", 2, 3, 4, 4, true},
		{"the count wins over a stale receipt", 2, 9, 3, 7, true},
		{"unchanged count is silent", 5, 5, 0, 0, false},
		{"a read mailbox is silent", 5, 3, 0, 0, false},
		{"a read mailbox still announces a receipt", 5, 3, 1, 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, raise := notifyDelta(tc.prev, true, tc.total, tc.delivered)
			if raise != tc.wantRaise || got != tc.wantCount {
				t.Fatalf("notifyDelta(%d, true, %d, %d) = (%d, %v), want (%d, %v)",
					tc.prev, tc.total, tc.delivered, got, raise, tc.wantCount, tc.wantRaise)
			}
		})
	}
}
