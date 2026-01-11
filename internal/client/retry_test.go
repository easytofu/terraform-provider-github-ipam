// Copyright (c) EasyTofu
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewRetryConfig(t *testing.T) {
	config := NewRetryConfig(5, 100)

	if config.MaxRetries != 5 {
		t.Errorf("expected MaxRetries 5, got %d", config.MaxRetries)
	}
	if config.BaseDelay != 100*time.Millisecond {
		t.Errorf("expected BaseDelay 100ms, got %v", config.BaseDelay)
	}
	if config.MaxDelay != 5*time.Second {
		t.Errorf("expected MaxDelay 5s, got %v", config.MaxDelay)
	}
	if config.JitterPct != 0.5 {
		t.Errorf("expected JitterPct 0.5, got %v", config.JitterPct)
	}
}

func TestWithRetry_SuccessFirstAttempt(t *testing.T) {
	config := NewRetryConfig(3, 10)
	attempts := 0

	err := WithRetry(context.Background(), config, func(ctx context.Context, attempt int) (bool, error) {
		attempts++
		return false, nil // Success, no retry
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestWithRetry_SuccessAfterRetry(t *testing.T) {
	config := NewRetryConfig(3, 10)
	attempts := 0

	err := WithRetry(context.Background(), config, func(ctx context.Context, attempt int) (bool, error) {
		attempts++
		if attempts < 3 {
			return true, errors.New("conflict") // Retry
		}
		return false, nil // Success on 3rd attempt
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithRetry_MaxRetriesExceeded(t *testing.T) {
	config := NewRetryConfig(3, 10)
	attempts := 0

	err := WithRetry(context.Background(), config, func(ctx context.Context, attempt int) (bool, error) {
		attempts++
		return true, errors.New("conflict") // Always retry
	})

	if err == nil {
		t.Error("expected error after max retries")
	}
	if attempts != 4 { // Initial + 3 retries
		t.Errorf("expected 4 attempts (1 + 3 retries), got %d", attempts)
	}
}

func TestWithRetry_NoRetryOnNonRetryableError(t *testing.T) {
	config := NewRetryConfig(3, 10)
	attempts := 0

	err := WithRetry(context.Background(), config, func(ctx context.Context, attempt int) (bool, error) {
		attempts++
		return false, errors.New("non-retryable error") // Don't retry
	})

	if err == nil {
		t.Error("expected error")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	config := NewRetryConfig(10, 100)
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := WithRetry(ctx, config, func(ctx context.Context, attempt int) (bool, error) {
		attempts++
		return true, errors.New("conflict")
	})

	if err == nil {
		t.Error("expected error from context cancellation")
	}
	// Should have stopped before max retries due to context cancellation
	if attempts >= 10 {
		t.Errorf("expected fewer attempts due to cancellation, got %d", attempts)
	}
}

func TestWithRetry_ContextDeadline(t *testing.T) {
	config := NewRetryConfig(10, 100)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	attempts := 0

	err := WithRetry(ctx, config, func(ctx context.Context, attempt int) (bool, error) {
		attempts++
		return true, errors.New("conflict")
	})

	if err == nil {
		t.Error("expected error from context deadline")
	}
}

func TestWithRetry_ZeroRetries(t *testing.T) {
	config := NewRetryConfig(0, 10)
	attempts := 0

	err := WithRetry(context.Background(), config, func(ctx context.Context, attempt int) (bool, error) {
		attempts++
		return true, errors.New("conflict")
	})

	if err == nil {
		t.Error("expected error")
	}
	if attempts != 1 { // Only initial attempt
		t.Errorf("expected 1 attempt with 0 retries, got %d", attempts)
	}
}

func TestWithRetry_AttemptNumberCorrect(t *testing.T) {
	config := NewRetryConfig(3, 10)
	attemptNumbers := []int{}

	_ = WithRetry(context.Background(), config, func(ctx context.Context, attempt int) (bool, error) {
		attemptNumbers = append(attemptNumbers, attempt)
		if len(attemptNumbers) < 4 {
			return true, errors.New("retry")
		}
		return false, nil
	})

	expected := []int{0, 1, 2, 3}
	if len(attemptNumbers) != len(expected) {
		t.Errorf("expected %d attempts, got %d", len(expected), len(attemptNumbers))
	}
	for i, n := range attemptNumbers {
		if n != expected[i] {
			t.Errorf("attempt %d: expected %d, got %d", i, expected[i], n)
		}
	}
}

func TestCalculateBackoff_Basic(t *testing.T) {
	config := RetryConfig{
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  5 * time.Second,
		JitterPct: 0, // No jitter for deterministic testing
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond},
		{1, 200 * time.Millisecond},
		{2, 400 * time.Millisecond},
		{3, 800 * time.Millisecond},
		{4, 1600 * time.Millisecond},
		{5, 3200 * time.Millisecond},
		{6, 5 * time.Second}, // Capped at MaxDelay
		{7, 5 * time.Second}, // Still capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := config.CalculateBackoff(tt.attempt)
			if result != tt.expected {
				t.Errorf("calculateBackoff(attempt=%d) = %v, want %v", tt.attempt, result, tt.expected)
			}
		})
	}
}

func TestCalculateBackoff_WithJitter(t *testing.T) {
	config := RetryConfig{
		BaseDelay: 100 * time.Millisecond,
		MaxDelay:  5 * time.Second,
		JitterPct: 0.5,
	}

	// Run multiple times to check jitter is applied
	baseExpected := 100 * time.Millisecond
	minExpected := time.Duration(float64(baseExpected) * 0.5)  // -50%
	maxExpected := time.Duration(float64(baseExpected) * 1.5)  // +50%

	for i := 0; i < 100; i++ {
		result := config.CalculateBackoff(0)
		if result < minExpected || result > maxExpected {
			t.Errorf("calculateBackoff with jitter = %v, expected between %v and %v", result, minExpected, maxExpected)
		}
	}
}

func TestCalculateBackoff_MaxDelayCap(t *testing.T) {
	config := RetryConfig{
		BaseDelay: 1 * time.Second,
		MaxDelay:  2 * time.Second,
		JitterPct: 0,
	}

	// After a few attempts, should be capped at MaxDelay
	result := config.CalculateBackoff(10)
	if result != 2*time.Second {
		t.Errorf("expected cap at MaxDelay, got %v", result)
	}
}

func TestCalculateBackoff_ZeroBaseDelay(t *testing.T) {
	config := RetryConfig{
		BaseDelay: 0,
		MaxDelay:  5 * time.Second,
		JitterPct: 0,
	}

	result := config.CalculateBackoff(0)
	if result != 0 {
		t.Errorf("expected 0 delay with 0 base delay, got %v", result)
	}
}
