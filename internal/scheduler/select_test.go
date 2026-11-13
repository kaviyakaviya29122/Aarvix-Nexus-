package scheduler

import "testing"

func TestPrefersIdleHealthyWorker(t *testing.T) {
	cs := []Candidate{
		{ID: "busy", Capacity: 4, Active: 3, Success: 100},
		{ID: "idle", Capacity: 4, Active: 0, Success: 100},
	}
	if i := Select(cs, nil); cs[i].ID != "idle" {
		t.Fatalf("picked %s", cs[i].ID)
	}
}

func TestAvoidsWorkersThatAlreadyFailedTheJob(t *testing.T) {
	cs := []Candidate{
		{ID: "a", Capacity: 4, Active: 0, Success: 50},
		{ID: "b", Capacity: 4, Active: 2, Success: 50},
	}
	if i := Select(cs, map[string]bool{"a": true}); cs[i].ID != "b" {
		t.Fatalf("picked %s", cs[i].ID)
	}
}

func TestFallsBackToExcludedWorkerWhenItIsTheOnlyOption(t *testing.T) {
	cs := []Candidate{{ID: "a", Capacity: 2}}
	if Select(cs, map[string]bool{"a": true}) != 0 {
		t.Fatal("expected fallback to excluded worker")
	}
}

func TestReturnsMinusOneWhenAllFull(t *testing.T) {
	cs := []Candidate{{ID: "a", Capacity: 1, Active: 1}}
	if Select(cs, nil) != -1 {
		t.Fatal("expected -1")
	}
}

func TestUnreliableWorkerScoresLower(t *testing.T) {
	good := Candidate{ID: "g", Capacity: 4, Success: 95, Fail: 5}
	bad := Candidate{ID: "b", Capacity: 4, Success: 20, Fail: 80}
	if Score(good) <= Score(bad) {
		t.Fatal("reliable worker should score higher")
	}
}
