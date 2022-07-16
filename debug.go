package rate5

import "fmt"

func (q *Limiter) debugPrintf(format string, a ...interface{}) {
	q.debugMutex.RLock()
	defer q.debugMutex.RUnlock()
	if !q.debug {
		return
	}
	msg := fmt.Sprintf(format, a...)
	select {
	case q.debugChannel <- msg:
	default:
		println(msg)
	}
}

func (q *Limiter) setDebugEvict() {
	q.Patrons.OnEvicted(func(src string, count interface{}) {
		q.debugPrintf("ratelimit (expired): %s | last count [%d]", src, count)
	})
}

func (q *Limiter) SetDebug(on bool) {
	q.debugMutex.Lock()
	if !on {
		q.debug = false
		q.Patrons.OnEvicted(nil)
		q.debugMutex.Unlock()
		return
	}
	q.debug = on
	q.setDebugEvict()
	q.debugMutex.Unlock()
	q.debugPrintf("rate5 debug enabled")
}

// DebugChannel enables debug mode and returns a channel where debug messages are sent.
// NOTE: You must read from this channel if created via this function or it will block
func (q *Limiter) DebugChannel() chan string {
	defer func() {
		q.debugMutex.Lock()
		q.debug = true
		q.debugMutex.Unlock()
	}()
	q.debugMutex.RLock()
	if q.debugChannel != nil {
		q.debugMutex.RUnlock()
		return q.debugChannel
	}
	q.debugMutex.RUnlock()
	q.debugMutex.Lock()
	defer q.debugMutex.Unlock()
	q.debugChannel = make(chan string, 25)
	q.setDebugEvict()
	return q.debugChannel
}
