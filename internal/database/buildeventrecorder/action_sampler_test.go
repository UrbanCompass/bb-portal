package buildeventrecorder

import (
	"math/rand"
	"testing"
)

func TestFirstNSampler(t *testing.T) {
	s := &firstNSampler{cap: 3}
	var got []int
	for i := 0; i < 6; i++ {
		got = append(got, s.admit())
	}
	want := []int{0, 1, 2, -1, -1, -1}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("admit #%d = %d, want %d (all: %v)", i+1, got[i], want[i], got)
		}
	}
}

func TestReservoirSamplerFillsThenSamples(t *testing.T) {
	// Deterministic source: always draws 0, so every post-cap action
	// displaces slot 0.
	s := &reservoirSampler{cap: 3, intn: func(int) int { return 0 }}
	for i := 0; i < 3; i++ {
		if got := s.admit(); got != i {
			t.Fatalf("fill phase admit #%d = %d, want %d", i+1, got, i)
		}
	}
	if got := s.admit(); got != 0 {
		t.Fatalf("post-cap admit with draw 0 = %d, want 0", got)
	}
	// A draw >= cap rejects the action.
	s.intn = func(n int) int { return n - 1 }
	if got := s.admit(); got != -1 {
		t.Fatalf("post-cap admit with draw n-1 = %d, want -1", got)
	}
}

func TestReservoirSamplerZeroCap(t *testing.T) {
	s := &reservoirSampler{cap: 0, intn: rand.Intn}
	for i := 0; i < 10; i++ {
		if got := s.admit(); got != -1 {
			t.Fatalf("cap 0 admit = %d, want -1", got)
		}
	}
}

// TestReservoirSamplerIsUniform checks that late arrivals are as likely to be
// selected as early ones, which is the property a first-N gate lacks.
func TestReservoirSamplerIsUniform(t *testing.T) {
	const (
		cap    = 10
		total  = 100
		trials = 20000
	)
	rng := rand.New(rand.NewSource(1))
	counts := make([]int, total)
	for trial := 0; trial < trials; trial++ {
		s := &reservoirSampler{cap: cap, intn: rng.Intn}
		slots := make([]int, cap)
		for i := 0; i < total; i++ {
			if slot := s.admit(); slot >= 0 {
				slots[slot] = i
			}
		}
		for _, id := range slots {
			counts[id]++
		}
	}
	want := float64(trials) * cap / total // 2000
	for id, c := range counts {
		if float64(c) < want*0.85 || float64(c) > want*1.15 {
			t.Errorf("action %d selected %d times, want ~%.0f", id, c, want)
		}
	}
}

func TestActionSamplingConfigFromEnv(t *testing.T) {
	tests := []struct {
		name         string
		strategy     string
		cap          string
		wantStrategy string
		wantCap      int
	}{
		{"defaults", "", "", "reservoir", 100},
		{"first with cap", "first", "25", "first", 25},
		{"explicit reservoir", "reservoir", "7", "reservoir", 7},
		{"unknown strategy falls back", "bogus", "", "reservoir", 100},
		{"non-numeric cap falls back", "", "lots", "reservoir", 100},
		{"negative cap falls back", "", "-5", "reservoir", 100},
		{"zero cap disables success sampling", "", "0", "reservoir", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(ActionSampleStrategyEnv, tc.strategy)
			t.Setenv(ActionSampleCapEnv, tc.cap)
			cfg := actionSamplingConfigFromEnv()
			if cfg.strategy != tc.wantStrategy || cfg.cap != tc.wantCap {
				t.Errorf("got {%s %d}, want {%s %d}", cfg.strategy, cfg.cap, tc.wantStrategy, tc.wantCap)
			}
		})
	}
}
