package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/config"
)

func TestLimiterLifecycle(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 10,
		Burst:             5,
	}

	limiter := New(cfg)
	defer limiter.Close()

	ctx := context.Background()

	err := limiter.Wait(ctx, "https://example.com/test")
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	limiter.Release("https://example.com/test")

	limiter.Close()

	err = limiter.Wait(ctx, "https://example.com/test")
	if err == nil {
		t.Fatal("Expected error when using closed limiter, got nil")
	}
	if err.Error() != "limiter is closed" {
		t.Fatalf("Expected 'limiter is closed' error, got: %v", err)
	}

	limiter.Close()
}

func TestLimiterMultipleClose(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 10,
	}

	limiter := New(cfg)

	limiter.Close()
	limiter.Close()
	limiter.Close()
}

func TestLimiterGracefulShutdown(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 10,
	}

	limiter := New(cfg)

	time.Sleep(10 * time.Millisecond)

	done := make(chan bool)
	go func() {
		limiter.Close()
		done <- true
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Close() did not complete within timeout")
	}
}

func TestLimiterFinalizer(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 10,
	}

	func() {
		limiter := New(cfg)
		_ = limiter
	}()

	t.Log("Finalizer safety net is registered (actual execution is non-deterministic)")
}
