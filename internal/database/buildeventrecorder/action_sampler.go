package buildeventrecorder

import (
	"hash/fnv"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"

	bes "github.com/bazelbuild/bazel/src/main/java/com/google/devtools/build/lib/buildeventstream/proto"
)

const (
	// ActionSampleStrategyEnv selects how successful actions are sampled.
	// See RegisterActionSampleStrategy for the supported values.
	ActionSampleStrategyEnv = "BB_PORTAL_ACTION_SAMPLE_STRATEGY"
	// ActionSampleCapEnv sets the maximum number of successful actions
	// stored per invocation. 0 stores no successful actions.
	ActionSampleCapEnv = "BB_PORTAL_ACTION_SAMPLE_CAP"

	defaultActionSampleStrategy = "random"
	defaultActionSampleCap      = 100
)

// ActionSampleStrategy decides which successful actions of an invocation
// are kept. Every successful action is stored with the key returned by
// SampleKey. Whenever an invocation holds more successful actions than the
// cap, the action with the highest key (the most recent one on ties) is
// deleted, so the stored sample is always the cap lowest keys seen so far.
//
// Because the sample is fully described by the stored keys, strategies
// carry no per-invocation state, and the sample survives BES reconnects and
// transaction retries without any bookkeeping.
type ActionSampleStrategy interface {
	SampleKey(id *bes.BuildEventId_ActionCompletedId, action *bes.ActionExecuted) int64
}

var actionSampleStrategies = map[string]ActionSampleStrategy{}

// RegisterActionSampleStrategy makes a strategy selectable through
// ActionSampleStrategyEnv. It is meant to be called from init functions.
func RegisterActionSampleStrategy(name string, strategy ActionSampleStrategy) {
	actionSampleStrategies[name] = strategy
}

// ActionSampleStrategyNames returns the names of all registered strategies.
func ActionSampleStrategyNames() []string {
	names := make([]string, 0, len(actionSampleStrategies))
	for name := range actionSampleStrategies {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func init() {
	RegisterActionSampleStrategy("random", randomActionSampleStrategy{})
	RegisterActionSampleStrategy("consistent", consistentActionSampleStrategy{})
	RegisterActionSampleStrategy("first", firstActionSampleStrategy{})
}

// actionSamplingConfig is the sampling configuration of a recorder.
type actionSamplingConfig struct {
	strategy ActionSampleStrategy
	cap      int
}

// actionSamplingConfigFromEnv reads the sampling configuration from the
// environment. Unset or invalid values fall back to the defaults, so a typo
// can never disable ingestion.
func actionSamplingConfigFromEnv() actionSamplingConfig {
	cfg := actionSamplingConfig{
		strategy: actionSampleStrategies[defaultActionSampleStrategy],
		cap:      defaultActionSampleCap,
	}
	if s, ok := actionSampleStrategies[os.Getenv(ActionSampleStrategyEnv)]; ok {
		cfg.strategy = s
	}
	if v, err := strconv.Atoi(os.Getenv(ActionSampleCapEnv)); err == nil && v >= 0 {
		cfg.cap = v
	}
	return cfg
}

// randomActionSampleStrategy gives every successful action an equal chance
// of being in the final sample, regardless of when it ran.
type randomActionSampleStrategy struct{}

func (randomActionSampleStrategy) SampleKey(*bes.BuildEventId_ActionCompletedId, *bes.ActionExecuted) int64 {
	return rand.Int63()
}

// consistentActionSampleStrategy derives the key from the action's identity
// (label, mnemonic and primary output). Like random sampling it has no bias
// towards early actions, but invocations of the same build keep largely the
// same actions, which makes their samples comparable.
type consistentActionSampleStrategy struct{}

func (consistentActionSampleStrategy) SampleKey(id *bes.BuildEventId_ActionCompletedId, action *bes.ActionExecuted) int64 {
	h := fnv.New64a()
	h.Write([]byte(id.GetLabel()))
	h.Write([]byte{0})
	h.Write([]byte(action.GetType()))
	h.Write([]byte{0})
	h.Write([]byte(id.GetPrimaryOutput()))
	return int64(h.Sum64() & math.MaxInt64)
}

// firstActionSampleStrategy keeps the first successful actions of an
// invocation. It is deterministic but biased towards early actions in the
// build graph.
type firstActionSampleStrategy struct{}

func (firstActionSampleStrategy) SampleKey(*bes.BuildEventId_ActionCompletedId, *bes.ActionExecuted) int64 {
	return 0
}
