package core

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"sync"
	"time"

	sdkcore "github.com/bizshuk/agentsdk/core"
)

// BurstSuppressor is a core.ObservationSource wrapper that drops percepts whose
// fingerprint matches the most recent emission within the cooldown
// window. Mirrors the TS version of log_doctor: a 12-char sha1 of
// (rule-id + text), per-rule cooldown to absorb bursts.
//
// Fingerprint = sha1(ruleId + "|" + payload)[:12]. A different rule or
// different text resets the cooldown — the same alarm shape from
// different files still goes through.
type BurstSuppressor struct {
	Inner    sdkcore.ObservationSource
	RuleID   string
	Cooldown time.Duration

	mu    sync.Mutex
	last  string
	until time.Time
}

// NewDedupe wires a Source through BurstSuppressor. RuleID distinguishes this
// rule from other listeners in the same process.
func NewBurstSuppressor(inner sdkcore.ObservationSource, ruleID string, cooldown time.Duration) *BurstSuppressor {
	return &BurstSuppressor{Inner: inner, RuleID: ruleID, Cooldown: cooldown}
}

// Name reports the wrapper's logical name.
func (d *BurstSuppressor) Name() string { return "dedupe:" + d.RuleID }

// Percepts is a fan-in proxy: it forwards percepts from Inner, but
// suppresses the (ruleId, text) pair when the cooldown is still active.
func (d *BurstSuppressor) Observations(ctx context.Context) <-chan sdkcore.Observation {
	src := d.Inner.Observations(ctx)
	out := make(chan sdkcore.Observation, 32)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case p, ok := <-src:
				if !ok {
					return
				}
				if d.shouldEmit(p) {
					select {
					case out <- p:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out
}

// shouldEmit returns true if the percept's fingerprint is fresh or the
// cooldown window has elapsed; otherwise false. Updates last / until on
// every call — recent matches are NOT re-emitted just because they
// didn't go through (avoids replay-storm on bursty sources).
func (d *BurstSuppressor) shouldEmit(p sdkcore.Observation) bool {
	fp := fingerprint(d.RuleID, payloadToString(p.Payload))
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	if fp == d.last && now.Before(d.until) {
		return false
	}
	d.last = fp
	if d.Cooldown > 0 {
		d.until = now.Add(d.Cooldown)
	} else {
		d.until = time.Time{}
	}
	return true
}

func fingerprint(ruleID, payload string) string {
	h := sha1.Sum([]byte(ruleID + "|" + payload))
	return hex.EncodeToString(h[:])[:12]
}

func payloadToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case nil:
		return ""
	}
	return ""
}

// LastFingerprint returns the most recently observed fingerprint —
// useful for tests and for diagnostics ("what did dedupe just skip?").
func (d *BurstSuppressor) LastFingerprint() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.last
}

// ShouldEmitForTest exposes shouldEmit for tests. Production callers
// never use this — the gating is automatic inside Observations().
func (d *BurstSuppressor) ShouldEmitForTest(p sdkcore.Observation) bool {
	return d.shouldEmit(p)
}