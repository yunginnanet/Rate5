package rate5

import (
	"crypto/rand"
	"encoding/binary"
	"runtime"
	"sync"
	"testing"
	"time"
)

var (
	dummyTicker *ticker
	stopDebug   = make(chan bool)
)

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
	var keylen = 10
	buf := make([]byte, keylen)
	for n := 0; n != keylen; n++ {
		buf[n] = charset[randomUint32()%uint32(len(charset))]
	}
	rp.key = string(buf)
}

var forCoverage = &sync.Once{}

func watchDebug(r *Limiter, t *testing.T) {
	t.Logf("debug enabled")
	rd := r.DebugChannel()
	forCoverage.Do(func() { rd = r.DebugChannel() })
	pre := "[Rate5] "
	for {
		select {
		case msg := <-rd:
			t.Logf("%s Limit: %s \n", pre, msg)
		case <-stopDebug:
			return
		}
	}
}

type ticker struct{}

func (tick *ticker) UniqueKey() string {
	return "tick"
}

func Test_NewDefaultLimiter(t *testing.T) {
	limiter := NewDefaultLimiter()
	limiter.Check(dummyTicker)
	if limiter.Peek(dummyTicker) {
		t.Errorf("Should not have been limited")
	}
	for n := 0; n != DefaultBurst+1; n++ {
		limiter.Check(dummyTicker)
	}
	if !limiter.Peek(dummyTicker) {
		t.Errorf("Should have been limited")
	}
}

func Test_NewLimiter(t *testing.T) {
	limiter := NewLimiter(5, 1)
	limiter.Check(dummyTicker)
	if limiter.Peek(dummyTicker) {
		t.Errorf("Should not have been limited")
	}
	limiter.Check(dummyTicker)
	if !limiter.Peek(dummyTicker) {
		t.Errorf("Should have been limited")
	}
}

func Test_NewCustomLimiter(t *testing.T) {
	limiter := NewCustomLimiter(Policy{
		Window: 5,
		Burst:  10,
		Strict: false,
	})

	go watchDebug(limiter, t)
	time.Sleep(25 * time.Millisecond)

	for n := 0; n < 9; n++ {
		limiter.Check(dummyTicker)
	}
	if limiter.Peek(dummyTicker) {
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should not have been limited. Ratelimiter count: %d", ct)
		} else {
			t.Fatalf("dummyTicker does not exist in ratelimiter at all!")
		}
	}
	if !limiter.Check(dummyTicker) {
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should have been limited. Ratelimiter count: %d", ct)
		} else {
			t.Fatalf("dummyTicker does not exist in ratelimiter at all!")
		}
	}

	stopDebug <- true
	limiter = nil
}

func Test_NewDefaultStrictLimiter(t *testing.T) {
	// DefaultBurst = 25
	// DefaultWindow = 5
	limiter := NewDefaultStrictLimiter()

	go watchDebug(limiter, t)
	time.Sleep(25 * time.Millisecond)

	for n := 0; n < 24; n++ {
		limiter.Check(dummyTicker)
	}

	if limiter.Peek(dummyTicker) {
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should not have been limited. Ratelimiter count: %d", ct)
		} else {
			t.Fatalf("dummyTicker does not exist in ratelimiter at all!")
		}
	}
	if !limiter.Check(dummyTicker) {
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should have been limited. Ratelimiter count: %d, policy: %d", ct, limiter.Ruleset.Burst)
		} else {
			t.Errorf("dummyTicker does not exist in ratelimiter at all!")
		}
	}

	stopDebug <- true
	limiter = nil
}

func Test_NewStrictLimiter(t *testing.T) {
	limiter := NewStrictLimiter(5, 1)
	limiter.Check(dummyTicker)
	if limiter.Peek(dummyTicker) {
		t.Errorf("Should not have been limited")
	}
	limiter.Check(dummyTicker)
	if !limiter.Peek(dummyTicker) {
		t.Errorf("Should have been limited")
	}
	limiter.Check(dummyTicker)
	// for coverage
	exp := limiter.DebugChannel()
	<-exp
	if limiter.Peek(dummyTicker) {
		t.Errorf("Should not have been limited")
	}
}

func concurrentTest(t *testing.T, jobs int, iterCount int, burst int64, shouldLimit bool) {
	var randos map[int]*randomPatron
	randos = make(map[int]*randomPatron)

	limiter := NewCustomLimiter(Policy{
		Window: 240,
		Burst:  burst,
		Strict: true,
	})

	limitNotice := sync.Once{}

	limiter.SetDebug(false)

	usedkeys := make(map[string]interface{})

	for n := 0; n != jobs; n++ {
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

	t.Logf("generated %d Patrons with unique keys, running Check() with them %d times concurrently with a burst limit of %d...",
		len(randos), iterCount, burst)

	finChan := make(chan bool, jobs*iterCount)
	var finished = 0

	for _, rp := range randos {
		go func(randomp *randomPatron) {
			for n := 0; n != iterCount; n++ {
				limiter.Check(randomp)
				if limiter.Peek(randomp) {
					limitNotice.Do(func() {
						t.Logf("(sync.Once) %s limited", randomp.UniqueKey())
					})
				}
				finChan <- true
			}
		}(rp)
	}

testloop:
	for {
		select {
		// case msg := <-limiter.DebugChannel():
		//	t.Logf("[debug] %s", msg)
		case <-finChan:
			finished++
		default:
			if finished >= (jobs * iterCount) {
				break testloop
			}
		}
	}

	for _, rp := range randos {
		var ok bool
		var ci interface{}
		if ci, ok = limiter.Patrons.Get(rp.UniqueKey()); !ok {
			t.Fatal("randomPatron does not exist in ratelimiter at all!")
		}
		ct := ci.(int64)
		if limiter.Peek(rp) && !shouldLimit {
			t.Logf("(%d goroutines running)", runtime.NumGoroutine())
			// runtime.Breakpoint()
			t.Errorf("FAIL: %s should not have been limited. Ratelimiter count: %d, policy: %d",
				rp.UniqueKey(), ct, limiter.Ruleset.Burst)
			continue
		}
		if !limiter.Peek(rp) && shouldLimit {
			t.Logf("(%d goroutines running)", runtime.NumGoroutine())
			// runtime.Breakpoint()
			t.Errorf("FAIL: %s should have been limited. Ratelimiter count: %d, policy: %d",
				rp.UniqueKey(), ct, limiter.Ruleset.Burst)
		}
	}
}

func Test_ConcurrentShouldNotLimit(t *testing.T) {
	concurrentTest(t, 50, 20, 20, false)
	concurrentTest(t, 50, 50, 50, false)
}

func Test_ConcurrentShouldLimit(t *testing.T) {
	concurrentTest(t, 50, 21, 20, true)
	concurrentTest(t, 50, 51, 50, true)
}
