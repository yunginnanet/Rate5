package rate5

import (
	"testing"
	"time"
)

var (
	dummyTicker *ticker
	testDebug   = true
)

func watchDebug(r *Limiter, t *testing.T) {
	t.Logf("debug enabled")
	rd := r.DebugChannel()

	pre := "[Rate5] "
	var lastcount = 0
	var count = 0
	for {
		select {
		case msg := <-rd:
			t.Logf("%s Limit: %s \n", pre, msg)
		default:
			count++
			if count-lastcount >= 10 {
				lastcount = count
				t.Logf("Times limited: %d", r.GetGrandTotalRated())
			}
			time.Sleep(time.Duration(10) * time.Millisecond)
		}
	}
}

type ticker struct{}

func init() {
	dummyTicker = &ticker{}
}

func (tick *ticker) UniqueKey() string {
	return "tick"
}

func Test_NewCustomLimiter(t *testing.T) {
	limiter := NewCustomLimiter(Policy{
		Window: 5,
		Burst:  10,
		Strict: false,
	})

	//goland:noinspection GoBoolExpressions
	if testDebug {
		go watchDebug(limiter, t)
		time.Sleep(100 * time.Millisecond)
	}

	for n := 0; n < 9; n++ {
		limiter.Check(dummyTicker)
	}
	if limiter.Peek(dummyTicker) {
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should not have been limited. Ratelimiter count: %d", ct)
		} else {
			t.Errorf("dummyTicker does not exist in ratelimiter at all!")
		}
	}
	if !limiter.Check(dummyTicker) {
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should have been limited. Ratelimiter count: %d", ct)
		} else {
			t.Errorf("dummyTicker does not exist in ratelimiter at all!")
		}
	}

	//goland:noinspection GoBoolExpressions
	if testDebug {
		t.Logf("[Finished NewCustomLimiter] Times ratelimited: %d", limiter.GetGrandTotalRated())
	}
}

func Test_NewDefaultStrictLimiter(t *testing.T) {
	// DefaultBurst = 25
	// DefaultWindow = 5
	limiter := NewDefaultStrictLimiter()

	//goland:noinspection GoBoolExpressions
	if testDebug {
		go watchDebug(limiter, t)
		time.Sleep(100 * time.Millisecond)
	}

	for n := 0; n < 24; n++ {
		limiter.Check(dummyTicker)
	}
	if limiter.Peek(dummyTicker) {
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should not have been limited. Ratelimiter count: %d", ct)
		} else {
			t.Errorf("dummyTicker does not exist in ratelimiter at all!")
		}
	}
	if !limiter.Check(dummyTicker) {
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should have been limited. Ratelimiter count: %d, policy: %d", ct, limiter.Ruleset.Burst)
		} else {
			t.Errorf("dummyTicker does not exist in ratelimiter at all!")
		}
	}

	//goland:noinspection GoBoolExpressions
	if testDebug {
		t.Logf("[Finished NewCustomLimiter] Times ratelimited: %d", limiter.GetGrandTotalRated())
	}
}
