package rate5

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/patrickmn/go-cache"
)

/*NewDefaultLimiter returns a ratelimiter with default settings without Strict mode.
 * Default window: 25 seconds
 * Default burst: 25 requests */
func NewDefaultLimiter() *Limiter {
	return newLimiter(Policy{
		Window: DefaultWindow,
		Burst:  DefaultBurst,
		Strict: false,
	})
}

// NewCustomLimiter returns a ratelimiter with the given Policy applied as the Ruleset.
func NewCustomLimiter(policy Policy) *Limiter {
	return newLimiter(policy)
}

/*NewLimiter returns a custom limiter witout Strict mode.
 * Window is the time in seconds that the limiter will cache requests.
 * Burst is the number of requests that can be made in the window.*/
func NewLimiter(window int, burst int) *Limiter {
	return newLimiter(Policy{
		Window: int64(window),
		Burst:  int64(burst),
		Strict: false,
	})
}

/*NewDefaultStrictLimiter returns a ratelimiter with default settings with Strict mode.
 * Default window: 25 seconds
 * Default burst: 25 requests */
func NewDefaultStrictLimiter() *Limiter {
	return newLimiter(Policy{
		Window: DefaultWindow,
		Burst:  DefaultBurst,
		Strict: true,
	})
}

/*NewStrictLimiter returns a custom limiter with Strict mode enabled.
 * Window is the time in seconds that the limiter will cache requests.
 * Burst is the number of requests that can be made in the window.*/
func NewStrictLimiter(window int, burst int) *Limiter {
	return newLimiter(Policy{
		Window: int64(window),
		Burst:  int64(burst),
		Strict: true,
	})
}

/*
NewHardcoreLimiter returns a custom limiter with Strict + Hardcore modes enabled.

Hardcore mode causes the time limited to be multiplied by the number of hits.
This differs from strict mode which is only using addition instead of multiplication.
*/
func NewHardcoreLimiter(window int, burst int) *Limiter {
	l := NewStrictLimiter(window, burst)
	l.Ruleset.Hardcore = true
	return l
}

func (q *Limiter) ResetItem(from Identity) {
	q.Patrons.Delete(from.UniqueKey())
	q.debugPrintf("ratelimit for %s has been reset", from.UniqueKey())
}

func newLimiter(policy Policy) *Limiter {
	window := time.Duration(policy.Window) * time.Second
	return &Limiter{
		Ruleset:    policy,
		Patrons:    cache.New(window, time.Duration(policy.Window)*time.Second),
		known:      make(map[interface{}]*int64),
		RWMutex:    &sync.RWMutex{},
		debugMutex: &sync.RWMutex{},
		debug:      DebugDisabled,
	}
}

func intPtr(i int64) *int64 {
	return &i
}

func (q *Limiter) getHitsPtr(src string) *int64 {
	q.RLock()
	if _, ok := q.known[src]; ok {
		oldPtr := q.known[src]
		q.RUnlock()
		return oldPtr
	}
	q.RUnlock()
	q.Lock()
	newPtr := intPtr(0)
	q.known[src] = newPtr
	q.Unlock()
	return newPtr
}

func (q *Limiter) strictLogic(src string, count int64) {
	knownHits := q.getHitsPtr(src)
	atomic.AddInt64(knownHits, 1)
	var extwindow int64
	prefix := "hardcore"
	switch {
	case q.Ruleset.Hardcore && q.Ruleset.Window > 1:
		extwindow = atomic.LoadInt64(knownHits) * q.Ruleset.Window
	case q.Ruleset.Hardcore && q.Ruleset.Window <= 1:
		extwindow = atomic.LoadInt64(knownHits) * 2
	case !q.Ruleset.Hardcore:
		prefix = "strict"
		extwindow = atomic.LoadInt64(knownHits) + q.Ruleset.Window
	}
	exttime := time.Duration(extwindow) * time.Second
	_ = q.Patrons.Replace(src, count, exttime)
	q.debugPrintf("%s ratelimit for %s: last count %d. time: %s", prefix, src, count, exttime)
}

func (q *Limiter) CheckStringer(from fmt.Stringer) bool {
	targ := IdentityStringer{stringer: from}
	return q.Check(targ)
}

// Check checks and increments an Identities UniqueKey() output against a list of cached strings to determine and raise it's ratelimitting status.
func (q *Limiter) Check(from Identity) (limited bool) {
	var count int64
	var err error
	src := from.UniqueKey()
	count, err = q.Patrons.IncrementInt64(src, 1)
	if err != nil {
		// IncrementInt64 should only error if the value is not an int64, so we can assume it's a new key.
		q.debugPrintf("ratelimit %s (new) ", src)
		// We can't reproduce this throwing an error, we can only assume that the key is new.
		_ = q.Patrons.Add(src, int64(1), time.Duration(q.Ruleset.Window)*time.Second)
		return false
	}
	if count < q.Ruleset.Burst {
		return false
	}
	if q.Ruleset.Strict {
		q.strictLogic(src, count)
	} else {
		q.debugPrintf("ratelimit %s: last count %d. time: %s",
			src, count, time.Duration(q.Ruleset.Window)*time.Second)
	}
	return true
}

// Peek checks an Identities UniqueKey() output against a list of cached strings to determine ratelimitting status without adding to its request count.
func (q *Limiter) Peek(from Identity) bool {
	q.Patrons.DeleteExpired()
	if ct, ok := q.Patrons.Get(from.UniqueKey()); ok {
		count := ct.(int64)
		if count > q.Ruleset.Burst {
			return true
		}
	}
	return false
}

func (q *Limiter) PeekStringer(from fmt.Stringer) bool {
	targ := IdentityStringer{stringer: from}
	return q.Peek(targ)
}
