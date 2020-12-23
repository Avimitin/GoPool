package pool

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPool_RegularCleanUp(t *testing.T) {
	p := NewPool(10, time.Second)
	p.applied = 10
	tCh := make(chan fnType, 1)
	for i := 0; i < 10; i++ {
		p.idling = append(p.idling, &Worker{
			task:         tCh,
			lastIdleTime: time.Now().Add(-time.Second),
		})
	}
	t.Logf("pool's worker amount: %d", len(p.idling))
	go p.RegularCleanUp()
	<-tCh
	ReleasePool(p)
	if len(p.idling) > 5 {
		t.Fatal("Unwanted result")
	}
}

func TestReleasePool(t *testing.T) {
	p := NewPool(10, time.Second)
	go ReleasePool(p)
	ticker := time.NewTicker(time.Second)
	select {
	case <-p.plumbing:
	case <-ticker.C:
		t.Fatal("Can't get data from p.plumbing")
	}
	if p.release == 1 {
		t.Fatal("Close pool failed")
	}
}

func TestPool_Employ(t *testing.T) {
	p := NewPool(10, time.Second)
	w := p.Employ()
	if p.applied != 1 {
		t.Fatal("Apply fail")
	}
	if w == nil {
		t.Fatal("Get Worker fail")
	}
	if w.plumbing == nil {
		t.Fatal("Worker's pipe is not initialize")
	}
	if w.task == nil {
		t.Fatal("Worker's task channel is not initialize")
	}
}

func TestPool_RevertWorker(t *testing.T) {
	p := NewPool(10, time.Second)
	p.plumbing <- &Worker{}
	// Let goroutine exit
	p.plumbing <- nil
	if len(p.idling) == 0 {
		t.Fatal("Revert worker failed")
	}
}

func TestPool_DetachWorker(t *testing.T) {
	p := NewPool(10, time.Second)
	w1 := p.DetachWorker()
	if w1 == nil {
		t.Fatal("Detach worker failed")
	}
	p.plumbing <- w1
	if len(p.idling) == 0 {
		t.Fatal("Revert worker failed")
	}
	w2 := p.DetachWorker()
	if w2 == nil {
		t.Fatal("Detach worker failed")
	}
	if w1 != w2 {
		t.Fatal("Reuse worker failed")
	}
	// clean
	p.plumbing <- nil
}

func TestPool_Submit(t *testing.T) {
	p := NewPool(10, time.Second)
	ticker := time.NewTicker(time.Second)
	ch := make(chan int32)
	p.Submit(func() { ch <- 1 })
	select {
	case <-ticker.C:
		t.Fatal("Time limit is up still don't get anything from channel")
	case <-ch:
	}
}

func TestPool_BinarySearchWorker(t *testing.T) {
	p := &Pool{expiry: time.Second}
	now := time.Now()
	p.idling = append(p.idling,
		&Worker{lastIdleTime: now.Add(-3 * time.Second)},
		&Worker{lastIdleTime: now.Add(-3 * time.Second)},
		&Worker{lastIdleTime: now.Add(-2 * time.Second)},
		&Worker{lastIdleTime: now.Add(-2 * time.Second)},
		&Worker{lastIdleTime: now.Add(-1 * time.Second)},
		&Worker{lastIdleTime: now.Add(-1 * time.Second)},
		&Worker{lastIdleTime: now.Add(-1 * time.Second)},
		&Worker{lastIdleTime: now.Add(time.Second)},
		&Worker{lastIdleTime: now.Add(time.Second)},
		&Worker{lastIdleTime: now.Add(2 * time.Second)},
	)
	p.applied = 10
	if len(p.idling) != int(p.applied) {
		t.Fatal("Initialize failed")
	}
	index := p.binarySearchWorker()
	if index != 6 {
		t.Fatal("Binary search failed")
	}
}

var (
	in        = "foo bar"
	out       = make(chan bool)
	done      = make(chan int32)
	testTimes = 1000000
	testFn    = func() {
		result := strings.Contains(in, "foo")
		out <- result
	}
)

func BenchmarkGoRoutine(b *testing.B) {
	// Bench setup
	b.ReportAllocs()
	var wg sync.WaitGroup

	go func() {
		for {
			select {
			case <-out:
				wg.Done()
			case <-done:
				return
			}
		}
	}()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(testTimes)
		for j := 0; j < testTimes; j++ {
			go testFn()
		}
	}
	wg.Wait()
	b.StopTimer()
	done <- 0
}

func BenchmarkPool(b *testing.B) {
	// Bench setup
	b.ReportAllocs()
	var wg sync.WaitGroup
	p := NewPool(1000000, time.Second)

	go func() {
		for {
			select {
			case <-out:
				wg.Done()
			case <-done:
				return
			}
		}
	}()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(testTimes)
		for j := 0; j < testTimes; j++ {
			p.Submit(testFn)
		}
	}
	wg.Wait()
	b.StopTimer()
	done <- 0
}
