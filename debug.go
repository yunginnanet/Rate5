package rate5

import (
	"fmt"
	"sync/atomic"
)

func (q *Limiter) debugPrintf(format string, a ...interface{}) {
	if atomic.CompareAndSwapUint32(&q.debug, DebugDisabled, DebugDisabled) {
		return
	}
	msg := fmt.Sprintf(format, a...)
	select {
	case q.debugChannel <- msg:
		//
	default:
		// drop the message but increment the lost counter
		atomic.AddInt64(&q.debugLost, 1)
	}
}

func (q *Limiter) setDebugEvict() {
	q.Patrons.OnEvicted(func(src string, count interface{}) {
		q.debugPrintf("ratelimit (expired): %s | last count [%d]", src, count)
	})
}

func (q *Limiter) SetDebug(on bool) {
	switch on {
	case true:
		if atomic.CompareAndSwapUint32(&q.debug, DebugDisabled, DebugEnabled) {
			q.debugPrintf("rate5 debug enabled")
		}
	case false:
		atomic.CompareAndSwapUint32(&q.debug, DebugEnabled, DebugDisabled)
	}
}

// DebugChannel enables debug mode and returns a channel where debug messages are sent.
//
// NOTE: If you do not read from this channel, the debug messages will eventually be lost.
// If this happens,
func (q *Limiter) DebugChannel() chan string {
	defer func() {
		atomic.CompareAndSwapUint32(&q.debug, DebugDisabled, DebugEnabled)
	}()
	q.debugMutex.RLock()
	if q.debugChannel != nil {
		q.debugMutex.RUnlock()
		return q.debugChannel
	}
	q.debugMutex.RUnlock()
	q.debugMutex.Lock()
	defer q.debugMutex.Unlock()
	q.debugChannel = make(chan string, 55)
	q.setDebugEvict()
	return q.debugChannel
}
