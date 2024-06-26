package rate5

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/patrickmn/go-cache"
)

const (
	strictPrefix   = "strict"
	hardcorePrefix = "hardcore"
)

var _counters = &sync.Pool{
	New: func() interface{} {
		i := &atomic.Int64{}
		i.Store(0)
		return i
	},
}

func getCounter() *atomic.Int64 {
	got := _counters.Get().(*atomic.Int64)
	got.Store(0)
	return got
}

func putCounter(i *atomic.Int64) {
	_counters.Put(i)
}

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

// ResetItem removes an Identity from the limiter's cache.
// This effectively resets the rate limit for the Identity.
func (q *Limiter) ResetItem(from Identity) {
	q.Patrons.Delete(from.UniqueKey())
	q.debugPrintf(msgRateLimitedRst, from.UniqueKey())
}

func (q *Limiter) onEvict(src string, count interface{}) {
	q.debugPrintf(msgRateLimitExpired, src, count)
	putCounter(count.(*atomic.Int64))

}

func newLimiter(policy Policy) *Limiter {
	window := time.Duration(policy.Window) * time.Second
	q := &Limiter{
		Ruleset:    policy,
		Patrons:    cache.New(window, time.Duration(policy.Window)*time.Second),
		known:      make(map[interface{}]*atomic.Int64),
		RWMutex:    &sync.RWMutex{},
		debugMutex: &sync.RWMutex{},
		debug:      DebugDisabled,
	}
	q.Patrons.OnEvicted(q.onEvict)
	return q
}

func intPtr(i int64) *atomic.Int64 {
	a := getCounter()
	a.Store(i)
	return a
}

func (q *Limiter) getHitsPtr(src string) *atomic.Int64 {
	q.RLock()
	if _, ok := q.known[src]; ok {
		oldPtr := q.known[src]
		q.RUnlock()
		return oldPtr
	}
	q.RUnlock()
	q.Lock()
	newPtr := getCounter()
	q.known[src] = newPtr
	q.Unlock()
	return newPtr
}

func (q *Limiter) strictLogic(src string, count *atomic.Int64) {
	knownHits := q.getHitsPtr(src)
	knownHits.Add(1)
	var extwindow int64
	prefix := hardcorePrefix
	switch {
	case q.Ruleset.Hardcore && q.Ruleset.Window > 1:
		extwindow = knownHits.Load() * q.Ruleset.Window
	case q.Ruleset.Hardcore && q.Ruleset.Window <= 1:
		extwindow = knownHits.Load() * 2
	case !q.Ruleset.Hardcore:
		prefix = strictPrefix
		extwindow = knownHits.Load() + q.Ruleset.Window
	}
	exttime := time.Duration(extwindow) * time.Second
	_ = q.Patrons.Replace(src, count, exttime)
	q.debugPrintf(msgRateLimitStrict, prefix, src, count.Load(), exttime)
}

func (q *Limiter) CheckStringer(from fmt.Stringer) bool {
	targ := IdentityStringer{stringer: from}
	return q.Check(targ)
}

// Check checks and increments an Identities UniqueKey() output against a list of cached strings to determine and raise it's ratelimitting status.
func (q *Limiter) Check(from Identity) (limited bool) {
	var count int64
	aval, ok := q.Patrons.Get(from.UniqueKey())
	switch {
	case !ok:
		q.debugPrintf(msgRateLimitedNew, from.UniqueKey())
		aval = intPtr(1)
		// We can't reproduce this throwing an error, we can only assume that the key is new.
		_ = q.Patrons.Add(from.UniqueKey(), aval, time.Duration(q.Ruleset.Window)*time.Second)
		return false
	case aval != nil:
		count = aval.(*atomic.Int64).Add(1)
		if count < q.Ruleset.Burst {
			return false
		}
	}
	if q.Ruleset.Strict {
		q.strictLogic(from.UniqueKey(), aval.(*atomic.Int64))
		return true
	}
	q.debugPrintf(msgRateLimited, from.UniqueKey(), count, time.Duration(q.Ruleset.Window)*time.Second)
	return true
}

// Peek checks an Identities UniqueKey() output against a list of cached strings to determine ratelimitting status without adding to its request count.
func (q *Limiter) Peek(from Identity) bool {
	if ct, ok := q.Patrons.Get(from.UniqueKey()); ok {
		count := ct.(*atomic.Int64).Load()
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
