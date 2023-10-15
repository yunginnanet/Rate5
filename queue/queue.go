package queue

import (
	"fmt"
	"sync"
	"sync/atomic"

	"git.tcp.direct/kayos/common/list"
	"golang.org/x/exp/constraints"
)

type Item[P constraints.Ordered, V any] struct {
	value    *V
	priority P
	index    int
}

func NewItem[P constraints.Ordered, V any](v *V, p P) *Item[P, V] {
	return &Item[P, V]{
		value:    v,
		priority: p,
		index:    0,
	}
}

func (i *Item[P, V]) Less(j any) bool {
	return i.priority < j.(Item[P, V]).priority
}

func (i *Item[P, V]) Value() *V {
	return i.value
}

func (i *Item[P, V]) Priority() P {
	return i.priority
}

func (i *Item[P, V]) Set(v *V) {
	i.value = v
}

func (i *Item[P, V]) SetPriority(p P) {
	i.priority = p
}

type boundedQueue[P constraints.Ordered, V any] struct {
	items []*Item[P, V]
	qcap  int64
	qlen  *atomic.Int64
	mu    *sync.RWMutex
	errs  *list.LockingList
}

type BoundedQueue[P constraints.Ordered, V any] struct {
	// making sure container/heap can access boundedQueue without the normal API being exposed to the user
	inner *boundedQueue[P, V]
}

func NewBoundedQueue[P constraints.Ordered, V any](cap int64) *BoundedQueue[P, V] {
	bq := &BoundedQueue[P, V]{}
	bq.inner = &boundedQueue[P, V]{
		items: make([]*Item[P, V], 0, cap),
		qcap:  cap,
		qlen:  &atomic.Int64{},
		mu:    &sync.RWMutex{},
		errs:  list.New(),
	}
	bq.inner.qlen.Store(0)
	return bq
}

func (b *boundedQueue[P, V]) err(err error) {
	b.errs.PushBack(err)
}

func (b *BoundedQueue[P, V]) Err() error {
	errLen := b.inner.errs.Len()
	switch errLen {
	case 0:
		return nil
	case 1:
		return b.inner.errs.Front().Value().(error)
	default:
		return fmt.Errorf("%w | (%d more errors in queue)", b.inner.errs.Front().Value().(error), errLen-1)
	}
}

func (b *BoundedQueue[P, V]) Len() int {
	return int(b.inner.qlen.Load())
}

func (b *boundedQueue[P, V]) Less(i, j int) bool {
	b.mu.RLock()
	less := b.items[P, V][i].priority < b.items[P, V][j].priority
	b.mu.RUnlock()
	return less
}

func (b *boundedQueue[P, V]) Swap(i, j int) {
	b.mu.Lock()
	b.items[P, V][i], b.items[P, V][j] = b.items[P, V][j], b.items[P, V][i]
	b.items[P, V][i].index = i
	b.items[P, V][j].index = j
	b.mu.Unlock()
}

func (b *boundedQueue[P, V]) Push(x any) {
	if b.qlen.Load() >= b.qcap {
		b.err(fmt.Errorf("%w: %v dropped", ErrQueueFull, x))
		return
	}
	defer b.qlen.Add(1)
	b.mu.Lock()
	b.items[P, V] = append(b.items[P, V], x.(*Item[P, V]))
	b.qlen.Add(1)
	b.mu.Unlock()
}

func (b *boundedQueue[P, V]) Pop() any {
	// todo
	return nil
}
