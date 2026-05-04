// Package worker provides a background job processor.
package worker

import (
	"errors"
	"math"
	"time"
)

// RetryPolicy defines how job failures are retried.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// NewRetryPolicy creates a RetryPolicy with sensible defaults.
// maxAttempts: total number of attempts (1 means no retries).
// baseDelay:   initial wait between attempts (doubles on each retry).
func NewRetryPolicy(maxAttempts int, baseDelay time.Duration) *RetryPolicy {
	return &RetryPolicy{
		MaxAttempts: maxAttempts,
		BaseDelay:   baseDelay,
		MaxDelay:    30 * time.Second,
	}
}

// ShouldRetry returns true if the job should be retried after this attempt.
// attempt is 1-indexed (first execution = attempt 1).
func (r *RetryPolicy) ShouldRetry(attempt int, err error) bool {
	if err == nil {
		return false
	}
	if attempt >= r.MaxAttempts {
		return false
	}
	var permanent *PermanentError
	return !errors.As(err, &permanent)
}

// Delay returns how long to wait before attempt+1.
func (r *RetryPolicy) Delay(attempt int) time.Duration {
	d := ExponentialBackoff(attempt, r.BaseDelay)
	if d > r.MaxDelay {
		return r.MaxDelay
	}
	return d
}

// PermanentError wraps an error that should not be retried.
type PermanentError struct {
	Cause error
}

func (e *PermanentError) Error() string { return "permanent: " + e.Cause.Error() }
func (e *PermanentError) Unwrap() error { return e.Cause }

// ExponentialBackoff returns BaseDelay * 2^(attempt-1), capped at MaxDelay.
// attempt is 1-indexed. The first retry (attempt=1) returns baseDelay itself.
func ExponentialBackoff(attempt int, base time.Duration) time.Duration {
	if attempt <= 1 {
		return base
	}
	multiplier := math.Pow(2, float64(attempt-1))
	return time.Duration(float64(base) * multiplier)
}
