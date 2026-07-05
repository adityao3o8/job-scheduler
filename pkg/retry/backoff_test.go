package retry

import (
	"testing"
	"time"
)

func TestNextDelay_Fixed(t *testing.T) {
	base := 5 * time.Second

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 5 * time.Second},
		{2, 5 * time.Second},
		{3, 5 * time.Second},
		{10, 5 * time.Second},
	}
	for _, tt := range tests {
		got := NextDelay("fixed", base, time.Hour, tt.attempt, false)
		if got != tt.want {
			t.Errorf("fixed attempt=%d: got %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestNextDelay_Linear(t *testing.T) {
	base := 5 * time.Second

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 15 * time.Second},
		{5, 25 * time.Second},
	}
	for _, tt := range tests {
		got := NextDelay("linear", base, time.Hour, tt.attempt, false)
		if got != tt.want {
			t.Errorf("linear attempt=%d: got %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestNextDelay_Exponential(t *testing.T) {
	base := 5 * time.Second

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 5 * time.Second},   // 5 * 2^0 = 5
		{2, 10 * time.Second},  // 5 * 2^1 = 10
		{3, 20 * time.Second},  // 5 * 2^2 = 20
		{4, 40 * time.Second},  // 5 * 2^3 = 40
		{5, 80 * time.Second},  // 5 * 2^4 = 80
		{6, 160 * time.Second}, // 5 * 2^5 = 160
	}
	for _, tt := range tests {
		got := NextDelay("exponential", base, time.Hour, tt.attempt, false)
		if got != tt.want {
			t.Errorf("exponential attempt=%d: got %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestNextDelay_ExponentialCap(t *testing.T) {
	base := 5 * time.Second
	maxDelay := 30 * time.Second

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 5 * time.Second},  // 5 < 30 → 5
		{2, 10 * time.Second}, // 10 < 30 → 10
		{3, 20 * time.Second}, // 20 < 30 → 20
		{4, 30 * time.Second}, // 40 > 30 → capped to 30
		{5, 30 * time.Second}, // 80 > 30 → capped to 30
	}
	for _, tt := range tests {
		got := NextDelay("exponential", base, maxDelay, tt.attempt, false)
		if got != tt.want {
			t.Errorf("exponential capped attempt=%d: got %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestNextDelay_LinearCap(t *testing.T) {
	base := 10 * time.Second
	maxDelay := 25 * time.Second

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 10 * time.Second},
		{2, 20 * time.Second},
		{3, 25 * time.Second}, // 30 > 25 → capped
		{4, 25 * time.Second}, // 40 > 25 → capped
	}
	for _, tt := range tests {
		got := NextDelay("linear", base, maxDelay, tt.attempt, false)
		if got != tt.want {
			t.Errorf("linear capped attempt=%d: got %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestNextDelay_Jitter(t *testing.T) {
	base := 10 * time.Second
	// With jitter, result should be in [base, base + base/2)
	// Run 100 times and verify bounds.
	for i := range 100 {
		got := NextDelay("fixed", base, time.Hour, 1, true)
		if got < base {
			t.Fatalf("jitter run %d: delay %v < base %v", i, got, base)
		}
		upperBound := base + base/2
		if got >= upperBound {
			t.Fatalf("jitter run %d: delay %v >= upper bound %v", i, got, upperBound)
		}
	}
}

func TestNextDelay_ExponentialJitter(t *testing.T) {
	base := 5 * time.Second
	// Attempt 3 → base delay = 20s, jitter adds [0, 10s)
	for i := range 100 {
		got := NextDelay("exponential", base, time.Hour, 3, true)
		if got < 20*time.Second {
			t.Fatalf("exp jitter run %d: delay %v < 20s", i, got)
		}
		if got >= 30*time.Second {
			t.Fatalf("exp jitter run %d: delay %v >= 30s", i, got)
		}
	}
}

func TestNextDelay_ZeroAttempt(t *testing.T) {
	// attempt < 1 is clamped to 1
	got := NextDelay("exponential", 5*time.Second, time.Hour, 0, false)
	if got != 5*time.Second {
		t.Errorf("zero attempt: got %v, want 5s", got)
	}
}

func TestNextDelay_UnknownStrategy(t *testing.T) {
	got := NextDelay("unknown", 5*time.Second, time.Hour, 1, false)
	if got != 5*time.Second {
		t.Errorf("unknown strategy: got %v, want 5s (fallback to base)", got)
	}
}
