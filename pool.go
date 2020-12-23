package pool

import (
	"sync"
	"sync/atomic"
	"time"
)

type fnType func()

type Pool struct {
	capacity int32
	applied  int32
	lock     sync.Mutex
	cond     *sync.Cond
	idling   []*Worker
	expiry   time.Duration
	release  int32 // 1 -> RUNNING 0 -> RELEASED
	plumbing chan *Worker
}

func NewPool(cap int32, expTime time.Duration) *Pool {
	p := &Pool{
		capacity: cap,
		expiry:   expTime,
		release:  1,
		plumbing: make(chan *Worker),
	}
	p.cond = sync.NewCond(&p.lock)
	go p.RegularCleanUp()
	go p.RevertWorker()
	return p
}

func ReleasePool(p *Pool) {
	atomic.CompareAndSwapInt32(&p.release, 1, 0)
	p.plumbing <- nil
}

func (p *Pool) binarySearchWorker() int {
	lo, hi := 0, len(p.idling)-1
	now := time.Now()
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if now.Sub(p.idling[mid].lastIdleTime) < p.expiry {
			hi = mid - 1
		} else {
			lo = mid + 1
		}
	}
	return hi
}

func (p *Pool) RegularCleanUp() {
	ticker := time.NewTicker(p.expiry)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if atomic.LoadInt32(&p.release) == 0 {
				return
			}
			p.lock.Lock()
			if p.applied == 0 || len(p.idling) == 0 {
				p.lock.Unlock()
				continue
			}
			var idlingTooLong []*Worker

			last := p.binarySearchWorker() //last is the last worker who idling over limit

			if last == len(p.idling)-1 {
				p.idling = p.idling[:0]
			} else {
				// amount is the amount of worker who just idl.
				amount := copy(p.idling, p.idling[last+1:])
				for j := amount; j < len(p.idling); j++ {
					p.idling[j] = nil
				}
				p.idling = p.idling[:amount]
			}
			p.applied -= int32(last + 1)
			// Notify blocking condition
			p.cond.Broadcast()
			p.lock.Unlock()

			// Worker's task channel may be block when they are working
			// So move them to a individual array for quicker unlock mutex
			for _, w := range idlingTooLong {
				w.task <- nil
			}
		}
	}
}

func (p *Pool) RevertWorker() {
	for {
		select {
		case w := <-p.plumbing:
			if w == nil {
				return
			}
			p.lock.Lock()
			p.idling = append(p.idling, w)
			p.lock.Unlock()
		}
	}
}

func (p *Pool) Submit(fn fnType) {
	w := p.DetachWorker()
	w.task <- fn
}

func (p *Pool) Employ() *Worker {
	atomic.AddInt32(&p.applied, 1)
	return &Worker{plumbing: p.plumbing, task: make(chan fnType, 1)}
}

func (p *Pool) DetachWorker() *Worker {
	var w *Worker
	p.lock.Lock()
	if len(p.idling) == 0 {
		if p.applied > p.capacity {
			// Wait for auto clean-up
			p.cond.Wait()
		}
		p.lock.Unlock()
		w = p.Employ()
		w.Do()
		return w
	}

	last := len(p.idling) - 1
	w = p.idling[last]
	p.idling[last] = nil
	p.idling = p.idling[:last]
	p.lock.Unlock()
	w.Do()
	return w
}
