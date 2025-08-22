package ai

import (
	"context"
	"testing"
)

type fakeProvider struct{ out string }

func (f fakeProvider) Name() string { return "fake" }
func (f fakeProvider) Chat(_ context.Context, _, _ string) (string, error) {
	return f.out, nil
}

func TestPrioritizeParsesScores(t *testing.T) {
	m := &Manager{provider: fakeProvider{out: `{"1": 5, "2": 2, "3": 7}`}}
	scores, err := m.Prioritize(context.Background(), []PriorityItem{
		{UID: 1}, {UID: 2}, {UID: 3},
	})
	if err != nil {
		t.Fatalf("prioritize: %v", err)
	}
	if scores[1] != 5 || scores[2] != 2 {
		t.Errorf("scores: %v", scores)
	}
	if scores[3] != 5 {
		t.Errorf("score must clamp to 5, got %d", scores[3])
	}
}

func TestPrioritizeToleratesFences(t *testing.T) {
	m := &Manager{provider: fakeProvider{out: "```json\n{\"7\": 4}\n```"}}
	scores, err := m.Prioritize(context.Background(), []PriorityItem{{UID: 7}})
	if err != nil {
		t.Fatalf("prioritize: %v", err)
	}
	if scores[7] != 4 {
		t.Errorf("scores: %v", scores)
	}
}

func TestPrioritizeDisabled(t *testing.T) {
	m := &Manager{}
	if _, err := m.Prioritize(context.Background(), []PriorityItem{{UID: 1}}); err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestPrioritizeGarbageResponse(t *testing.T) {
	m := &Manager{provider: fakeProvider{out: "sorry, no scores"}}
	if _, err := m.Prioritize(context.Background(), []PriorityItem{{UID: 1}}); err == nil {
		t.Fatal("expected error for garbage response")
	}
}

func TestInterpretSearch(t *testing.T) {
	m := &Manager{
		provider: fakeProvider{
			out: `{"keywords":["contract"],"from":"amy@example.com","to":"","subject":"","has_attachment":true,"before":"2026-01-31","after":"2026-01-01"}`,
		},
	}
	spec, err := m.InterpretSearch(context.Background(), "contracts with attachments from Amy in January")
	if err != nil {
		t.Fatalf("interpret: %v", err)
	}
	if len(spec.Keywords) != 1 || spec.Keywords[0] != "contract" {
		t.Errorf("keywords: %v", spec.Keywords)
	}
	if spec.From != "amy@example.com" || !spec.HasAttachment {
		t.Errorf("spec: %+v", spec)
	}
	if spec.Before != "2026-01-31" || spec.After != "2026-01-01" {
		t.Errorf("dates: %+v", spec)
	}
}

func TestInterpretSearchFences(t *testing.T) {
	m := &Manager{provider: fakeProvider{out: "```json\n{\"keywords\":[\"invoice\"]}\n```"}}
	spec, err := m.InterpretSearch(context.Background(), "invoices")
	if err != nil {
		t.Fatalf("interpret: %v", err)
	}
	if len(spec.Keywords) != 1 || spec.Keywords[0] != "invoice" {
		t.Errorf("spec: %+v", spec)
	}
}

func TestInterpretSearchDisabled(t *testing.T) {
	m := &Manager{}
	if _, err := m.InterpretSearch(context.Background(), "anything"); err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}
