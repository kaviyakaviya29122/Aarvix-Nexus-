package scheduler

import "math"

// Candidate is a live worker as seen by the scheduler.
type Candidate struct {
	ID         string
	Capacity   int
	Active     int     // jobs currently assigned or running
	Load       float64 // 0..1 host load reported by the worker
	AvgMS      float64 // moving average job time
	Success    int64
	Fail       int64
}

// Score ranks workers: free capacity (45%), historical success rate (30%),
// host load (-15%) and latency (-10%). Higher is better.
func Score(c Candidate) float64 {
	if c.Capacity <= 0 {
		return math.Inf(-1)
	}
	free := float64(c.Capacity-c.Active) / float64(c.Capacity)
	rate := float64(c.Success+1) / float64(c.Success+c.Fail+2)
	load := math.Min(math.Max(c.Load, 0), 1)
	lat := math.Min(c.AvgMS/5000, 1)
	return 0.45*free + 0.30*rate - 0.15*load - 0.10*lat
}

// Select returns the index of the best worker with spare capacity, preferring
// workers that have not already failed this job (exclude). If only excluded
// workers have capacity it falls back to them. Returns -1 if everything is full.
func Select(cs []Candidate, exclude map[string]bool) int {
	best, bestExcl := -1, -1
	for i, c := range cs {
		if c.Active >= c.Capacity {
			continue
		}
		if exclude[c.ID] {
			if bestExcl < 0 || better(c, cs[bestExcl]) {
				bestExcl = i
			}
			continue
		}
		if best < 0 || better(c, cs[best]) {
			best = i
		}
	}
	if best >= 0 {
		return best
	}
	return bestExcl
}

func better(a, b Candidate) bool {
	sa, sb := Score(a), Score(b)
	if sa != sb {
		return sa > sb
	}
	return a.ID < b.ID
}
