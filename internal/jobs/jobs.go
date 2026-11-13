package jobs

import (
	"encoding/json"
	"math"
	"math/rand"
	"time"
)

var Known = map[string]bool{
	"echo": true, "sleep": true, "compute": true, "business_op": true,
	"flaky": true, "fail": true, "crash": true,
}

// AdminOnly types exist to test failure handling; regular users may not submit them.
var AdminOnly = map[string]bool{"flaky": true, "fail": true, "crash": true}

// Message is what the scheduler publishes to a specific worker's queue.
type Message struct {
	JobID   string          `json:"job_id"`
	Fence   int64           `json:"fence"`
	Attempt int             `json:"attempt"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Backoff returns an exponential delay (2s,4s,8s..., capped at 60s) with +-20% jitter.
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := 2 * math.Pow(2, float64(attempt-1))
	if d > 60 {
		d = 60
	}
	return time.Duration(d * (0.8 + rand.Float64()*0.4) * float64(time.Second))
}
