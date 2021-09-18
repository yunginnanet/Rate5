package rate5

import (
	"fmt"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
)

// NewDefaultLimiter returns a ratelimiter with default settings without Strict mode
func NewDefaultLimiter() *Limiter {
	return newLimiter(Policy{
		Window: DefaultWindow,
		Burst:  DefaultBurst,
		Strict: false,
	})
}

// NewLimiter returns a custom limiter witout Strict mode
func NewLimiter(window int, burst int) *Limiter {
	return newLimiter(Policy{
		Window: window,
		Burst:  burst,
		Strict: false,
	})
}

// NewDefaultStrictLimiter returns a ratelimiter with default settings with Strict mode
func NewDefaultStrictLimiter() *Limiter {
	return newLimiter(Policy{
		Window: DefaultWindow,
		Burst:  DefaultBurst,
		Strict: true,
	})
}

// NewStrictLimiter returns a custom limiter with Strict mode
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
	q.known = make(map[interface{}]int)
	q.mu = &sync.RWMutex{}
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

func (q *Limiter) strictLogic(src string, count int) {
	q.mu.Lock()
	if _, ok := q.known[src]; !ok {
		q.known[src] = 1
	}

	q.known[src]++
	extwindow := q.Ruleset.Window + q.known[src]

	if err := q.Patrons.Replace(src, count, time.Duration(extwindow)*time.Second); err != nil {
		q.debugPrint("Rate5: " + err.Error())
	}
	q.mu.Unlock()
	q.debugPrint("ratelimit (strictly limited): ", count, " ", src)
	q.increment()
}

// Check checks and increments an Identities UniqueKey() output against a list of cached strings to determine and raise it's ratelimitting status
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

// Peek checks an Identities UniqueKey() output against a list of cached strings to determine ratelimitting status without adding to its request count
func (q *Limiter) Peek(from Identity) bool {
	if _, ok := q.Patrons.Get(from.UniqueKey()); ok {
		return true
	}

	return false
}

func (q *Limiter) increment() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.count++
}

// GetGrandTotalRated returns the historic total amount of times we have ever reported something as ratelimited
func (q *Limiter) GetGrandTotalRated() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.count
}

func (q *Limiter) debugPrint(a ...interface{}) {
	if q.Debug {
		debugChannel <- fmt.Sprint(a...)
	}
}
