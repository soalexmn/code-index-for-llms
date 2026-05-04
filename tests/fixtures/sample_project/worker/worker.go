// Package worker provides a background job processor.
package worker

import (
	"context"
	"fmt"
	"sync"
)

// Worker pulls jobs from a JobQueue and processes them concurrently.
type Worker struct {
	queue       *JobQueue
	concurrency int
	done        chan struct{}
	wg          sync.WaitGroup
	handler     func(Job) error
}

// NewWorker constructs a Worker backed by the provided queue.
// concurrency controls how many jobs are processed simultaneously.
func NewWorker(q *JobQueue, concurrency int, handler func(Job) error) *Worker {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Worker{
		queue:       q,
		concurrency: concurrency,
		done:        make(chan struct{}),
		handler:     handler,
	}
}

// Start launches the worker goroutines and blocks until ctx is cancelled.
// Call Stop to signal a graceful shutdown, then wait for Start to return.
func (w *Worker) Start(ctx context.Context) {
	for i := 0; i < w.concurrency; i++ {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-w.done:
					return
				default:
					job, ok := w.queue.Dequeue()
					if !ok {
						continue
					}
					if err := w.processJob(job); err != nil {
						fmt.Printf("worker: job %s failed: %v\n", job.ID, err)
					}
				}
			}
		}()
	}
	w.wg.Wait()
}

// Stop signals all worker goroutines to exit after finishing their current job.
func (w *Worker) Stop() {
	close(w.done)
}

// processJob invokes the handler for a single job, recovering from panics.
func (w *Worker) processJob(job Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in job %s: %v", job.ID, r)
		}
	}()
	return w.handler(job)
}
