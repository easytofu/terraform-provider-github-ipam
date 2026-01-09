// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// RetryConfig holds configuration for exponential backoff retry logic.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	JitterPct  float64 // 0.0 to 1.0
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 10,
		BaseDelay:  200 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		JitterPct:  0.5,
	}
}

// NewRetryConfig creates a new retry configuration with custom values.
func NewRetryConfig(maxRetries int, baseDelayMs int64) RetryConfig {
	return RetryConfig{
		MaxRetries: maxRetries,
		BaseDelay:  time.Duration(baseDelayMs) * time.Millisecond,
		MaxDelay:   5 * time.Second,
		JitterPct:  0.5,
	}
}

// CalculateBackoff calculates the backoff duration for a given attempt.
// Uses exponential backoff: baseDelay * 2^attempt with jitter.
func (c *RetryConfig) CalculateBackoff(attempt int) time.Duration {
	// Exponential: baseDelay * 2^attempt
	backoff := float64(c.BaseDelay) * math.Pow(2, float64(attempt))

	// Cap at max delay
	if backoff > float64(c.MaxDelay) {
		backoff = float64(c.MaxDelay)
	}

	// Add jitter: +/- JitterPct of the backoff
	if c.JitterPct > 0 {
		jitterRange := backoff * c.JitterPct
		jitter := (rand.Float64()*2 - 1) * jitterRange // -jitterRange to +jitterRange
		backoff += jitter
	}

	// Ensure non-negative
	if backoff < 0 {
		backoff = float64(c.BaseDelay)
	}

	return time.Duration(backoff)
}

// RetryableFunc is a function that can be retried.
// Returns (shouldRetry, error). If shouldRetry is true and error is non-nil,
// the operation will be retried. If shouldRetry is false, the operation stops.
type RetryableFunc func(ctx context.Context, attempt int) (shouldRetry bool, err error)

// WithRetry executes a function with exponential backoff retry logic.
func WithRetry(ctx context.Context, config RetryConfig, fn RetryableFunc) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		shouldRetry, err := fn(ctx, attempt)
		if err == nil {
			return nil
		}
		lastErr = err

		if !shouldRetry {
			return err
		}

		if attempt < config.MaxRetries {
			backoff := config.CalculateBackoff(attempt)
			tflog.Warn(ctx, "Optimistic lock conflict, retrying", map[string]interface{}{
				"attempt":     attempt + 1,
				"max_retries": config.MaxRetries,
				"backoff_ms":  backoff.Milliseconds(),
			})

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}
	}

	return fmt.Errorf("exceeded max retries (%d): %w", config.MaxRetries, lastErr)
}
