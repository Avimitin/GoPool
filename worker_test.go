package pool

import (
	"testing"
)

func TestWorker(t *testing.T) {
	taskCh := make(chan fnType)
	workCh := make(chan *Worker)
	w1 := &Worker{
		task:     taskCh,
		plumbing: workCh,
	}
	w1.Do()

	var i int
	fn := func() { i++ }
	w1.task <- fn
	if i != 1 {
		t.Fatal("Worker fail to do jobs")
	}
	w2 := <-workCh
	if w1 != w2 {
		t.Fatal("Got unwanted worker addr")
	}
}
