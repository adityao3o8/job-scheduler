package retry

import (
	"math"
	"math/rand/v2"
	"time"
)

// NextDelay computes the retry delay for a given strategy.
//
//   - fixed:       delay = base
//   - linear:      delay = base * attempt
//   - exponential: delay = base * 2^(attempt-1), capped at maxDelay
//
// When jitter is true an additional random duration in [0, delay/2) is added to
// the computed delay to decorrelate retries across workers.
//
// attempt is 1-indexed (first retry = attempt 1).
func NextDelay(strategy string, base, maxDelay time.Duration, attempt int, jitter bool) time.Duration {
	if attempt < 1 {
		attempt = 1
	}

	var d time.Duration
	switch strategy {
	case "fixed":
		d = base
	case "linear":
		d = base * time.Duration(attempt)
	case "exponential":
		exp := math.Pow(2, float64(attempt-1))
		d = time.Duration(float64(base) * exp)
	default:
		d = base
	}

	if maxDelay > 0 && d > maxDelay {
		d = maxDelay
	}

	if jitter && d > 0 {
		d += time.Duration(rand.Int64N(int64(d / 2)))
	}
	return d
}

// NextRunAt returns the absolute time for the next retry: now + delay.
func NextRunAt(strategy string, base, maxDelay time.Duration, attempt int, jitter bool) time.Time {
	return time.Now().Add(NextDelay(strategy, base, maxDelay, attempt, jitter))
}
