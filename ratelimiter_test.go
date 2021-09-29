package rate5

import (
	"crypto/rand"
	"encoding/binary"
	"testing"
	"time"
)

var (
	dummyTicker *ticker
	testDebug   = true

	stopDebug chan bool
)

func init() {
	stopDebug = make(chan bool)
}

type randomPatron struct {
	key string
	Identity
}

const charset = "abcdefghijklmnopqrstuvwxyz1234567890"

func (rp *randomPatron) UniqueKey() string {
	return rp.key
}

func randomUint32() uint32 {
	b := make([]byte, 8192)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return binary.LittleEndian.Uint32(b)
}

func (rp *randomPatron) GenerateKey() {
	var keylen = 8
	buf := make([]byte, keylen)
	for n := 0; n != keylen; n++ {
		buf[n] = charset[randomUint32()%uint32(len(charset))]
	}
	rp.key = string(buf)
}

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
		case <-stopDebug:
			return
		default:
			count++
			if count-lastcount >= 10 {
				lastcount = count
				t.Logf("Times limited: %d", r.GetGrandTotalRated())
			}
			time.Sleep(10 * time.Millisecond)
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
		time.Sleep(25 * time.Millisecond)
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

	stopDebug <- true
	limiter = nil
}

func Test_NewDefaultStrictLimiter(t *testing.T) {
	// DefaultBurst = 25
	// DefaultWindow = 5
	limiter := NewDefaultStrictLimiter()

	//goland:noinspection GoBoolExpressions
	if testDebug {
		go watchDebug(limiter, t)
		time.Sleep(25 * time.Millisecond)
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

	stopDebug <- true
	limiter = nil
}

// This test is only here for safety, if the package is not safe, this will often panic.
// We give this a healthy amount of padding in terms of our checks as this is far beyond the tolerances we expect during runtime.
// At the end of the day, not panicing here is passing.
func Test_ConcurrentSafetyTest(t *testing.T) {
	var randos map[int]*randomPatron
	randos = make(map[int]*randomPatron)

	limiter := NewCustomLimiter(Policy{
		Window: 240,
		Burst:  5000,
		Strict: true,
	})

	usedkeys := make(map[string]interface{})

	for n := 0; n != 5000; n++ {
		randos[n] = new(randomPatron)
		ok := true
		for ok {
			randos[n].GenerateKey()
			_, ok = usedkeys[randos[n].key]
			if ok {
				t.Log("collision")
			}
		}
	}

	t.Logf("generated %d Patrons with unique keys", len(randos))

	doneChan := make(chan bool)
	finChan := make(chan bool)
	var finished = 0

	for _, rp := range randos {
		for n := 0; n != 5; n++ {
			go func() {
				limiter.Check(rp)
				limiter.Peek(rp)
				finChan <- true
			}()
		}
	}

	var done = false
	for {
		select {
		case <-finChan:
			finished++
		default:
			if finished == 25000 {
				done = true
				break
			}
		}
		if done {
			go func() {
				doneChan <- true
			}()
			break
		}
	}

	<-doneChan
	println("done")

	for _, rp := range randos {
		if limiter.Peek(rp) {
			if ct, ok := limiter.Patrons.Get(rp.UniqueKey()); ok {
				t.Logf("WARN: Should not have been limited. Ratelimiter count: %d, policy: %d", ct, limiter.Ruleset.Burst)
			} else {
				t.Errorf("randomPatron does not exist in ratelimiter at all!")
			}
		}
	}

	//goland:noinspection GoBoolExpressions
	if testDebug {
		t.Logf("[Finished StrictConcurrentStressTest] Times ratelimited: %d", limiter.GetGrandTotalRated())
	}
}
