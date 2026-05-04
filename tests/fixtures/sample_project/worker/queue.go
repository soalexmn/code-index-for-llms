// Package worker provides a background job processor.
package worker

import (
	"fmt"
	"sync"
	"time"
)

// Job represents a unit of background work.
type Job struct {
	ID      string
	Type    string
	Payload []byte
	Enqueued time.Time
	Attempts int
}

// JobQueue is a thread-safe in-memory FIFO queue for background jobs.
type JobQueue struct {
	mu       sync.Mutex
	items    []Job
	capacity int
}

// NewJobQueue creates a JobQueue with the specified maximum capacity.
// When the queue is full, Enqueue returns an error rather than blocking.
func NewJobQueue(capacity int) *JobQueue {
	return &JobQueue{
		items:    make([]Job, 0, capacity),
		capacity: capacity,
	}
}

// Enqueue adds a job to the back of the queue.
// Returns an error if the queue is at capacity.
func (q *JobQueue) Enqueue(job Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) >= q.capacity {
		return fmt.Errorf("queue is full (capacity %d)", q.capacity)
	}
	job.Enqueued = time.Now()
	q.items = append(q.items, job)
	return nil
}

// Dequeue removes and returns the front job.
// Returns (Job{}, false) if the queue is empty.
func (q *JobQueue) Dequeue() (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return Job{}, false
	}
	job := q.items[0]
	q.items = q.items[1:]
	return job, true
}

// Len returns the current number of jobs in the queue.
func (q *JobQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Peek returns the front job without removing it.
// Returns (Job{}, false) if the queue is empty.
func (q *JobQueue) Peek() (Job, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return Job{}, false
	}
	return q.items[0], true
}
