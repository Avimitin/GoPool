package pool

import "time"

type Worker struct {
	plumbing     chan *Worker
	task         chan fnType
	lastIdleTime time.Time
}

func (w *Worker) Rest() {
	w.plumbing <- w
}

func (w *Worker) Do() {
	go func() {
		for t := range w.task {
			if t == nil {
				return
			}
			t()
			w.Rest()
		}
	}()
}
