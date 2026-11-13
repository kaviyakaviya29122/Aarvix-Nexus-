package jobs

import (
	"testing"
	"time"
)

func TestBackoffGrowsAndCaps(t *testing.T) {
	for i := 0; i < 50; i++ {
		d1 := Backoff(1)
		if d1 < 1600*time.Millisecond || d1 > 2400*time.Millisecond {
			t.Fatalf("attempt 1 out of range: %v", d1)
		}
		if d := Backoff(20); d > 72*time.Second {
			t.Fatalf("backoff not capped: %v", d)
		}
	}
	if Backoff(0) <= 0 {
		t.Fatal("attempt 0 must be treated as 1")
	}
}
