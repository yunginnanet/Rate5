package rate5

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"runtime"
	"sync"
	"testing"
	"time"
)

var (
	dummyTicker *ticker
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

var (
	forCoverage     = &sync.Once{}
	watchDebugMutex = &sync.Mutex{}
)

func watchDebug(ctx context.Context, r *Limiter, t *testing.T) {
	watchDebugMutex.Lock()
	defer watchDebugMutex.Unlock()
	rd := r.DebugChannel()
	forCoverage.Do(func() {
		r.SetDebug(false)
		r.SetDebug(true)
		rd = r.DebugChannel()
	})
	for {
		select {
		case <-ctx.Done():
			r = nil
			return
		case msg := <-rd:
			t.Logf("%s \n", msg)
		default:
		}
	}
}

func peekCheckLimited(t *testing.T, limiter *Limiter, shouldbe, stringer bool) {
	limited := limiter.Peek(dummyTicker)
	if stringer {
		limited = limiter.PeekStringer(dummyTicker)
	}
	switch {
	case limited && !shouldbe:
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should not have been limited. Ratelimiter count: %d", ct)
		} else {
			t.Fatalf("dummyTicker does not exist in ratelimiter at all!")
		}
	case !limited && shouldbe:
		if ct, ok := limiter.Patrons.Get(dummyTicker.UniqueKey()); ok {
			t.Errorf("Should have been limited. Ratelimiter count: %d", ct)
		} else {
			t.Fatalf("dummyTicker does not exist in ratelimiter at all!")
		}
	case limited && shouldbe:
		t.Logf("dummyTicker is limited (pass).")
	case !limited && !shouldbe:
		t.Logf("dummyTicker is not limited (pass).")
	}
}

// this test exists here for coverage, we are simulating the debug channel overflowing and then invoking println().
func Test_debugPrintf(t *testing.T) {
	limiter := NewLimiter(1, 1)
	_ = limiter.DebugChannel()
	for n := 0; n < 50; n++ {
		limiter.Check(dummyTicker)
	}
}

type ticker struct{}

func (tick *ticker) UniqueKey() string {
	return "TestItem"
}

func (tick *ticker) String() string {
	return "TestItem"
}

func Test_ResetItem(t *testing.T) {
	limiter := NewLimiter(500, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go watchDebug(ctx, limiter, t)
	time.Sleep(25 * time.Millisecond)
	for n := 0; n < 10; n++ {
		limiter.Check(dummyTicker)
	}
	limiter.ResetItem(dummyTicker)
	peekCheckLimited(t, limiter, false, false)
	cancel()
}

func Test_NewDefaultLimiter(t *testing.T) {
	limiter := NewDefaultLimiter()
	limiter.Check(dummyTicker)
	peekCheckLimited(t, limiter, false, false)
	for n := 0; n != DefaultBurst; n++ {
		limiter.Check(dummyTicker)
	}
	peekCheckLimited(t, limiter, true, false)
}

func Test_CheckAndPeekStringer(t *testing.T) {
	limiter := NewDefaultLimiter()
	limiter.CheckStringer(dummyTicker)
	peekCheckLimited(t, limiter, false, true)
	for n := 0; n != DefaultBurst; n++ {
		limiter.CheckStringer(dummyTicker)
	}
	peekCheckLimited(t, limiter, true, true)
}

func Test_NewLimiter(t *testing.T) {
	limiter := NewLimiter(5, 1)
	limiter.Check(dummyTicker)
	peekCheckLimited(t, limiter, false, false)
	limiter.Check(dummyTicker)
	peekCheckLimited(t, limiter, true, false)
}

func Test_NewDefaultStrictLimiter(t *testing.T) {
	limiter := NewDefaultStrictLimiter()
	ctx, cancel := context.WithCancel(context.Background())
	go watchDebug(ctx, limiter, t)
	time.Sleep(25 * time.Millisecond)
	for n := 0; n < 25; n++ {
		limiter.Check(dummyTicker)
	}
	peekCheckLimited(t, limiter, false, false)
	limiter.Check(dummyTicker)
	peekCheckLimited(t, limiter, true, false)
	cancel()
	limiter = nil
}

func Test_NewStrictLimiter(t *testing.T) {
	limiter := NewStrictLimiter(5, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go watchDebug(ctx, limiter, t)
	limiter.Check(dummyTicker)
	peekCheckLimited(t, limiter, false, false)
	limiter.Check(dummyTicker)
	peekCheckLimited(t, limiter, true, false)
	limiter.Check(dummyTicker)
	// for coverage, first we give the debug messages a couple seconds to be safe,
	// then we wait for the cache eviction to trigger a debug message.
	time.Sleep(2 * time.Second)
	t.Logf(<-limiter.DebugChannel())
	peekCheckLimited(t, limiter, false, false)
	for n := 0; n != 6; n++ {
		limiter.Check(dummyTicker)
	}
	peekCheckLimited(t, limiter, true, false)
	time.Sleep(5 * time.Second)
	peekCheckLimited(t, limiter, true, false)
	time.Sleep(8 * time.Second)
	peekCheckLimited(t, limiter, false, false)
	cancel()
	limiter = nil
}

func Test_NewHardcoreLimiter(t *testing.T) {
	limiter := NewHardcoreLimiter(1, 5)
	ctx, cancel := context.WithCancel(context.Background())
	go watchDebug(ctx, limiter, t)
	for n := 0; n != 4; n++ {
		limiter.Check(dummyTicker)
	}
	peekCheckLimited(t, limiter, false, false)
	if !limiter.Check(dummyTicker) {
		t.Errorf("Should have been limited")
	}
	t.Logf("limited once, waiting for cache eviction")
	time.Sleep(2 * time.Second)
	peekCheckLimited(t, limiter, false, false)
	for n := 0; n != 4; n++ {
		limiter.Check(dummyTicker)
	}
	peekCheckLimited(t, limiter, false, false)
	if !limiter.Check(dummyTicker) {
		t.Errorf("Should have been limited")
	}
	limiter.Check(dummyTicker)
	limiter.Check(dummyTicker)
	time.Sleep(3 * time.Second)
	peekCheckLimited(t, limiter, true, false)
	time.Sleep(5 * time.Second)
	peekCheckLimited(t, limiter, false, false)
	for n := 0; n != 4; n++ {
		limiter.Check(dummyTicker)
	}
	peekCheckLimited(t, limiter, false, false)
	for n := 0; n != 10; n++ {
		limiter.Check(dummyTicker)
	}
	time.Sleep(10 * time.Second)
	peekCheckLimited(t, limiter, true, false)
	cancel()
	// for coverage, triggering the switch statement case for hardcore logic
	limiter2 := NewHardcoreLimiter(2, 5)
	ctx2, cancel2 := context.WithCancel(context.Background())
	go watchDebug(ctx2, limiter2, t)
	for n := 0; n != 6; n++ {
		limiter2.Check(dummyTicker)
	}
	peekCheckLimited(t, limiter2, true, false)
	time.Sleep(4 * time.Second)
	peekCheckLimited(t, limiter2, false, false)
	cancel2()
}

func concurrentTest(t *testing.T, jobs int, iterCount int, burst int64, shouldLimit bool) { //nolint:funlen
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

func Test_debugChannelOverflow(t *testing.T) {
	limiter := NewDefaultLimiter()
	_ = limiter.DebugChannel()
	for n := 0; n != 78; n++ {
		limiter.Check(dummyTicker)
		if limiter.debugLost > 0 {
			t.Fatalf("debug channel overflowed")
		}
	}
	limiter.Check(dummyTicker)
	if limiter.debugLost == 0 {
		t.Fatalf("debug channel did not overflow")
	}
}
