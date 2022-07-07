package rate5

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/patrickmn/go-cache"
)

// NewDefaultLimiter returns a ratelimiter with default settings without Strict mode.
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

// NewLimiter returns a custom limiter witout Strict mode
func NewLimiter(window int, burst int) *Limiter {
	return newLimiter(Policy{
		Window: int64(window),
		Burst:  int64(burst),
		Strict: false,
	})
}

// NewDefaultStrictLimiter returns a ratelimiter with default settings with Strict mode.
func NewDefaultStrictLimiter() *Limiter {
	return newLimiter(Policy{
		Window: DefaultWindow,
		Burst:  DefaultBurst,
		Strict: true,
	})
}

// NewStrictLimiter returns a custom limiter with Strict mode.
func NewStrictLimiter(window int, burst int) *Limiter {
	return newLimiter(Policy{
		Window: int64(window),
		Burst:  int64(burst),
		Strict: true,
	})
}

func newLimiter(policy Policy) *Limiter {
	return &Limiter{
		Ruleset:    policy,
		Patrons:    cache.New(time.Duration(policy.Window)*time.Second, 5*time.Second),
		known:      make(map[interface{}]*int64),
		RWMutex:    &sync.RWMutex{},
		debugMutex: &sync.RWMutex{},
		debug:      false,
	}
}

func (q *Limiter) SetDebug(on bool) {
	if !on {
		q.Patrons.OnEvicted(nil)
	}
	q.debugMutex.Lock()
	q.debug = on
	q.debugMutex.Unlock()
}

// DebugChannel enables debug mode and returns a channel where debug messages are sent.
// NOTE: You must read from this channel if created via this function or it will block
func (q *Limiter) DebugChannel() chan string {
	q.debugMutex.RLock()
	if q.debug {
		q.debugMutex.RUnlock()
		return q.debugChannel
	}
	q.debugMutex.RUnlock()
	q.debugMutex.Lock()
	q.debug = true
	q.debugChannel = make(chan string, 25)
	q.Patrons.OnEvicted(func(src string, count interface{}) {
		q.debugPrint("ratelimit (expired): ", src, " ", count)
	})
	q.debugMutex.Unlock()
	return q.debugChannel
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
	extwindow := q.Ruleset.Window + atomic.LoadInt64(knownHits)
	if err := q.Patrons.Replace(src, count, time.Duration(extwindow)*time.Second); err != nil {
		q.debugPrint("ratelimit (strict) error: " + err.Error())
	}
	q.debugPrint("ratelimit (strict) limited: ", count, " ", src)
}

// Check checks and increments an Identities UniqueKey() output against a list of cached strings to determine and raise it's ratelimitting status.
func (q *Limiter) Check(from Identity) (limited bool) {
	var count int64
	var err error
	src := from.UniqueKey()
	count, err = q.Patrons.IncrementInt64(src, 1)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			q.debugPrint("ratelimit (new): ", src)
			if cacheErr := q.Patrons.Add(src, int64(1), time.Duration(q.Ruleset.Window)*time.Second); cacheErr != nil {
				q.debugPrint("ratelimit error: " + cacheErr.Error())
			}
			return false
		}
		q.debugPrint("ratelimit error: " + err.Error())
		return true
	}
	if count < q.Ruleset.Burst {
		return false
	}
	if q.Ruleset.Strict {
		q.strictLogic(src, count)
	} else {
		q.debugPrint("ratelimit (limited): ", count, " ", src)
	}
	return true
}

// Peek checks an Identities UniqueKey() output against a list of cached strings to determine ratelimitting status without adding to its request count.
func (q *Limiter) Peek(from Identity) bool {
	if ct, ok := q.Patrons.Get(from.UniqueKey()); ok {
		count := ct.(int64)
		if count > q.Ruleset.Burst {
			return true
		}
	}
	return false
}

func (q *Limiter) debugPrint(a ...interface{}) {
	q.debugMutex.RLock()
	defer q.debugMutex.RUnlock()
	if !q.debug {
		return
	}
	go func(msg ...interface{}) {
		q.debugChannel <- fmt.Sprint(msg...)
	}(a)
}
