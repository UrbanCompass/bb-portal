package buildeventrecorder

import (
	"math/rand"
	"os"
	"strconv"
)

const (
	// ActionSampleStrategyEnv selects how successful actions are sampled.
	// Supported values are "reservoir" (default) and "first".
	ActionSampleStrategyEnv = "BB_PORTAL_ACTION_SAMPLE_STRATEGY"
	// ActionSampleCapEnv sets the maximum number of successful actions
	// persisted per invocation.
	ActionSampleCapEnv = "BB_PORTAL_ACTION_SAMPLE_CAP"

	defaultActionSampleCap = 100

	actionSampleStrategyReservoir = "reservoir"
	actionSampleStrategyFirst     = "first"
)

// actionSampler decides which successful actions are persisted for a single
// invocation. Implementations are not safe for concurrent use; every
// invocation owns its own sampler.
type actionSampler interface {
	// admit is called once per successful action, in arrival order. It
	// returns the slot (in [0, cap)) the action should occupy, or -1 if the
	// action should not be persisted. If the slot is already occupied, the
	// previous occupant must be evicted by the caller.
	admit() int
	// clone returns an independent copy of the sampler's current state, so
	// the state can be restored if the transaction that consumed slots rolls
	// back.
	clone() actionSampler
}

// actionSamplingConfig configures how successful actions are sampled.
type actionSamplingConfig struct {
	strategy string
	cap      int
}

// actionSamplingConfigFromEnv reads the sampling configuration from the
// environment. Unset or invalid values fall back to the defaults, so a typo
// can never disable ingestion.
func actionSamplingConfigFromEnv() actionSamplingConfig {
	cfg := actionSamplingConfig{
		strategy: actionSampleStrategyReservoir,
		cap:      defaultActionSampleCap,
	}
	switch s := os.Getenv(ActionSampleStrategyEnv); s {
	case actionSampleStrategyReservoir, actionSampleStrategyFirst:
		cfg.strategy = s
	}
	if v, err := strconv.Atoi(os.Getenv(ActionSampleCapEnv)); err == nil && v >= 0 {
		cfg.cap = v
	}
	return cfg
}

// newSampler builds a sampler for one invocation. A nil intn uses math/rand.
func (c actionSamplingConfig) newSampler(intn func(n int) int) actionSampler {
	if intn == nil {
		intn = rand.Intn
	}
	switch c.strategy {
	case actionSampleStrategyFirst:
		return &firstNSampler{cap: c.cap}
	default:
		return &reservoirSampler{cap: c.cap, intn: intn}
	}
}

// reservoirSampler implements Algorithm R: every successful action has an
// equal cap/n probability of being in the final sample, regardless of
// arrival order.
type reservoirSampler struct {
	cap  int
	seen int
	intn func(n int) int
}

func (s *reservoirSampler) admit() int {
	s.seen++
	if s.seen <= s.cap {
		return s.seen - 1
	}
	if j := s.intn(s.seen); j < s.cap {
		return j
	}
	return -1
}

// firstNSampler keeps the first cap successful actions. It is cheap and
// deterministic but biased towards early actions in the build graph.
type firstNSampler struct {
	cap  int
	seen int
}

func (s *firstNSampler) admit() int {
	s.seen++
	if s.seen <= s.cap {
		return s.seen - 1
	}
	return -1
}

func (s *reservoirSampler) clone() actionSampler {
	c := *s
	return &c
}

func (s *firstNSampler) clone() actionSampler {
	c := *s
	return &c
}
