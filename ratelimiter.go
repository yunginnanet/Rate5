package rate5

import (
	"fmt"
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
		Window: window,
		Burst:  burst,
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
		Window: window,
		Burst:  burst,
		Strict: true,
	})
}

func newLimiter(policy Policy) *Limiter {
	q := new(Limiter)
	q.Ruleset = policy
	q.Patrons = cache.New(time.Duration(q.Ruleset.Window)*time.Second, 5*time.Second)
	q.known = make(map[interface{}]rated)

	return q
}

// DebugChannel enables Debug mode and returns a channel where debug messages are sent (NOTE: You must read from this channel if created via this function or it will block)
func (q *Limiter) DebugChannel() chan string {
	q.Patrons.OnEvicted(func(src string, count interface{}) {
		q.debugPrint("ratelimit (expired): ", src, " ", count)
	})
	q.Debug = true
	debugChannel = make(chan string, 20)
	return debugChannel
}

func (s rated) inc() {
	for !atomic.CompareAndSwapUint32(&s.locker, stateUnlocked, stateLocked) {
		time.Sleep(10 * time.Millisecond)
	}
	defer atomic.StoreUint32(&s.locker, stateUnlocked)

	if s.seen.Load() == nil {
		s.seen.Store(1)
		return
	}
	s.seen.Store(s.seen.Load().(int) + 1)
}

func (q *Limiter) strictLogic(src string, count int) {
	for !atomic.CompareAndSwapUint32(&q.locker, stateUnlocked, stateLocked) {
		time.Sleep(10 * time.Millisecond)
	}
	defer atomic.StoreUint32(&q.locker, stateUnlocked)

	if _, ok := q.known[src]; !ok {
		q.known[src] = rated{
			seen:   &atomic.Value{},
			locker: stateUnlocked,
		}
	}
	q.known[src].inc()
	extwindow := q.Ruleset.Window + q.known[src].seen.Load().(int)
	if err := q.Patrons.Replace(src, count, time.Duration(extwindow)*time.Second); err != nil {
		q.debugPrint("Rate5: " + err.Error())
	}
	q.debugPrint("ratelimit (strictly limited): ", count, " ", src)
	q.increment()
}

// Check checks and increments an Identities UniqueKey() output against a list of cached strings to determine and raise it's ratelimitting status.
func (q *Limiter) Check(from Identity) bool {
	var count int
	var err error
	src := from.UniqueKey()
	if count, err = q.Patrons.IncrementInt(src, 1); err != nil {
		q.debugPrint("ratelimit (new): ", src)
		if err := q.Patrons.Add(src, 1, time.Duration(q.Ruleset.Window)*time.Second); err != nil {
			println("Rate5: " + err.Error())
		}
		return false
	}
	if count < q.Ruleset.Burst {
		return false
	}
	if !q.Ruleset.Strict {
		q.increment()
		q.debugPrint("ratelimit (limited): ", count, " ", src)
		return true
	}
	q.strictLogic(src, count)
	return true
}

// Peek checks an Identities UniqueKey() output against a list of cached strings to determine ratelimitting status without adding to its request count.
func (q *Limiter) Peek(from Identity) bool {
	if ct, ok := q.Patrons.Get(from.UniqueKey()); ok {
		count := ct.(int)
		if count > q.Ruleset.Burst {
			return true
		}
	}
	return false
}

func (q *Limiter) increment() {
	if q.count.Load() == nil {
		q.count.Store(1)
		return
	}
	q.count.Store(q.count.Load().(int) + 1)
}

// GetGrandTotalRated returns the historic total amount of times we have ever reported something as ratelimited.
func (q *Limiter) GetGrandTotalRated() int {
	if q.count.Load() == nil {
		return 0
	}
	return q.count.Load().(int)
}

func (q *Limiter) debugPrint(a ...interface{}) {
	if q.Debug {
		debugChannel <- fmt.Sprint(a...)
	}
}
