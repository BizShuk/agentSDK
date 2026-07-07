// Package harness hosts the policy middlewares (retry / budget / timeout).
// Each is a Middleware factory and matches the runtime.Loop Next signature.
package harness

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bizshuk/agentsdk/core"
	"github.com/bizshuk/agentsdk/middleware"
)

// RetryableError is the contract for "this error is transient". Model
// providers wrap transient failures (5xx, network) with this so the
// Retry middleware can re-dispatch.
//
// Stub providers (testutil.FakeProvider) do not return it; production
// adapters in M4 will.
type RetryableError interface {
	error
	Retryable() bool
}

// IsRetryable reports whether err satisfies RetryableError AND has
// Retryable()==true. errors.Is / errors.As chains work normally.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	var r RetryableError
	if errors.As(err, &r) {
		return r.Retryable()
	}
	return false
}

// RetryConfig configures the Retry middleware.
type RetryConfig struct {
	N           int           // max attempts (1 = no retry)
	BaseBackoff time.Duration // first sleep; doubles each retry
	MaxBackoff  time.Duration // cap on per-attempt sleep
	// Sleeper is injectable for tests. Defaults to time.Sleep.
	Sleeper func(d time.Duration)
}

// Retry returns a Middleware that retries transient errors.
//
// Retries happen at the inner-most layer of the chain — the dispatcher
// is re-invoked with the SAME effect (NOT a wrapped one). If the LLM has
// produced a partial response, the provider should not consider it
// committed until it returns successfully.
func Retry(cfg RetryConfig) middleware.Middleware {
	if cfg.N <= 0 {
		cfg.N = 1
	}
	if cfg.BaseBackoff == 0 {
		cfg.BaseBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 5 * time.Second
	}
	sleep := cfg.Sleeper
	if sleep == nil {
		sleep = time.Sleep
	}
	return func(next middleware.Next) middleware.Next {
		return func(ctx context.Context, state core.State, eff core.Instruction) (core.State, *core.Event, bool, error) {
			backoff := cfg.BaseBackoff
			var lastErr error
			for attempt := 0; attempt < cfg.N; attempt++ {
				if ctx.Err() != nil {
					return state, nil, false, ctx.Err()
				}
				s, in, term, err := next(ctx, state, eff)
				if err == nil {
					return s, in, term, nil
				}
				lastErr = err
				if !IsRetryable(err) {
					return state, nil, false, err
				}
				if attempt+1 >= cfg.N {
					break
				}
				sleep(backoff)
				if backoff < cfg.MaxBackoff {
					backoff *= 2
					if backoff > cfg.MaxBackoff {
						backoff = cfg.MaxBackoff
					}
				}
			}
			return state, nil, false, lastErr
		}
	}
}

// SimpleRetryable is a convenience error type — providers wrap their
// transient failures with this so IsRetryable works out of the box.
type SimpleRetryable struct {
	Reason string
}

// Error implements error.
func (e SimpleRetryable) Error() string { return "retryable: " + e.Reason }

// Retryable implements RetryableError.
func (e SimpleRetryable) Retryable() bool { return true }

// TransientError is an alias for transient infra errors that look like
// retryable but expose a class string (e.g. "rate_limit"). Use this when
// the underlying cause matters for telemetry.
type TransientError struct {
	Class string
	Cause error
}

// Error implements error.
func (e *TransientError) Error() string {
	if e.Cause == nil {
		return "transient(" + e.Class + ")"
	}
	return "transient(" + e.Class + "): " + e.Cause.Error()
}

// Unwrap exposes the cause for errors.Is / errors.As.
func (e *TransientError) Unwrap() error { return e.Cause }

// Retryable implements RetryableError.
func (e *TransientError) Retryable() bool { return true }

// Common error class strings. Providers can wrap with these so retries
// can be classified later.
const (
	RetryClassNetwork   = "network"
	RetryClassRateLimit = "rate_limit"
	RetryClassServer5xx = "server_5xx"
	RetryClassTimeout   = "timeout"
)

// Helper to detect error class substrings from wrapped errors — providers
// in M4 will set TransientError.Class explicitly; this stays for legacy
// strings already emitted by some SDKs.
func classifyByString(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "rate limit"):
		return RetryClassRateLimit
	case strings.Contains(msg, "timeout"):
		return RetryClassTimeout
	case strings.Contains(msg, "5xx") || strings.Contains(msg, "internal server"):
		return RetryClassServer5xx
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "network"):
		return RetryClassNetwork
	}
	return ""
}