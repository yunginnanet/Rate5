package ratelimit

import (
	"fmt"
	cache "github.com/patrickmn/go-cache"
	"sync"
	"time"
)

// NewDefaultLimiter returns a ratelimiter with default settings without Strict mode
func NewDefaultLimiter() *Limiter {
	return newLimiter(DefaultWindow, DefaultBurst, false)
}

// NewLimiter returns a custom limiter witout Strict mode
func NewLimiter(window int, burst int) *Limiter {
	return newLimiter(window, burst, false)
}

// NewDefaultStrictLimiter returns a ratelimiter with default settings with Strict mode
func NewDefaultStrictLimiter() *Limiter {
	return newLimiter(DefaultWindow, DefaultBurst, true)
}

// NewStrictLimiter returns a custom limiter with Strict mode
func NewStrictLimiter(window int, burst int) *Limiter {
	return newLimiter(window, burst, true)
}

func newLimiter(window int, burst int, strict bool) *Limiter {
	q := new(Limiter)
	q.Ruleset = Policy{
		Window: window,
		Burst:  burst,
		Strict: strict,
	}
	q.Patrons = cache.New(time.Duration(q.Ruleset.Window)*time.Second, 5*time.Second)
	q.known = make(map[interface{}]int)
	q.mu = &sync.Mutex{}
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

// Check checks an Identities UniqueKey() output against a list of cached strings to determine ratelimitting status
func (q *Limiter) Check(from Identity) bool {
	var (
		count int
		err   error
	)
	src := from.UniqueKey()
	if count, err = q.Patrons.IncrementInt(src, 1); err != nil {
		q.debugPrint("ratelimit (new): ", src)
		if err := q.Patrons.Add(src, 1, time.Duration(q.Ruleset.Window)*time.Second); err != nil {
			println("Rate5: " + err.Error())
		}
		return false
	}

	if count > q.Ruleset.Burst {
		if !q.Ruleset.Strict {
			q.debugPrint("ratelimit (limited): ", count, " ", src)
			return true
		}
		q.mu.Lock()
		if _, ok := q.known[src]; !ok {
			q.known[src] = 1
		}

		q.known[src]++
		extwindow := q.Ruleset.Window + q.known[src]
		q.mu.Unlock()
		if err := q.Patrons.Replace(src, count, time.Duration(extwindow)*time.Second); err != nil {
			println("Rate5: " + err.Error())
		}
		q.debugPrint("ratelimit (strictly limited): ", count, " ", src)
		return true
	}
	return false
}

func (q *Limiter) debugPrint(a ...interface{}) {
	if q.Debug {
		debugChannel <- fmt.Sprint(a...)
	}
}
