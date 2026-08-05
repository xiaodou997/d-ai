package asynctask

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// Options tunes one registered task type.
type Options struct {
	// MaxAttempts caps how many times a task may be claimed.
	//
	// The default is 1, and image generation should keep it. The engine refuses
	// to retry an attempt whose request_id already has a usage log, but there is
	// a window — upstream billed, log not yet committed, process dies — where
	// that guard cannot see the charge. At MaxAttempts 1 a crash fails the task
	// and the user resubmits, which is strictly safer than the status quo, where
	// startup recovery unconditionally re-ran every running task.
	//
	// Raise it only for work that is genuinely free to repeat.
	MaxAttempts int

	// TTL overrides the engine's default retention for this type. A Prepared may
	// override per task.
	TTL time.Duration
}

type registration struct {
	handler Handler
	opts    Options
}

// registry maps task types to handlers. It is frozen at Start.
type registry struct {
	mu      sync.RWMutex
	entries map[string]registration
	frozen  bool
}

func newRegistry() *registry {
	return &registry{entries: map[string]registration{}}
}

// register binds a handler to a task type.
//
// Registering twice, registering after Start, or registering an empty type or
// nil handler all panic. These are composition-root wiring mistakes, and every
// one of them is worse when discovered later: a duplicate silently shadows a
// capability, and a late registration produces rows no worker will ever claim
// (the claim query filters on the type set fixed at Start).
func (r *registry) register(taskType string, h Handler, opts Options) {
	if taskType == "" {
		panic("asynctask: cannot register an empty task type")
	}
	if h == nil {
		panic(fmt.Sprintf("asynctask: nil handler for task type %q", taskType))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		panic(fmt.Sprintf("asynctask: task type %q registered after Start; "+
			"the claimable type set is fixed when workers start", taskType))
	}
	if _, exists := r.entries[taskType]; exists {
		panic(fmt.Sprintf("asynctask: task type %q registered twice", taskType))
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 1
	}
	r.entries[taskType] = registration{handler: h, opts: opts}
}

// freeze closes the registry. Called once by Start.
func (r *registry) freeze() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
}

// lookup returns the registration for a task type.
func (r *registry) lookup(taskType string) (registration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.entries[taskType]
	return reg, ok
}

// types returns the registered task types, sorted for a stable claim query.
// This is the set a worker on this instance is willing to claim, which is why
// an instance that registers nothing never touches another instance's work.
func (r *registry) types() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.entries))
	for t := range r.entries {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
