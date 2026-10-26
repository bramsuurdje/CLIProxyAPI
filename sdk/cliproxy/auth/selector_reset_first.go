package auth

import (
	"context"
	"strconv"
	"strings"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

// ResetFirstSelector prefers the credential whose longest (weekly) quota window
// resets soonest, so quota that is about to expire is spent before quota that
// will still be there next week.
//
// Reset times come from the passive quota snapshot that each upstream response
// leaves in Auth.Quota.Signals (Claude and Codex). Credentials without a running
// window (never used, or their window already rolled over) rank last, because
// using them starts a fresh window. Ties keep fill-first's deterministic ID order,
// so providers without quota signals behave exactly like fill-first.
type ResetFirstSelector struct{}

// Pick selects the available auth whose weekly quota window resets soonest.
func (s *ResetFirstSelector) Pick(ctx context.Context, provider, model string, opts cliproxyexecutor.Options, auths []*Auth) (*Auth, error) {
	_ = opts
	now := time.Now()
	available, err := getSelectorAvailableAuths(ctx, auths, provider, model, now)
	if err != nil {
		return nil, err
	}
	available = preferCodexWebsocketAuths(ctx, provider, available)

	best := available[0]
	bestReset, bestKnown := weeklyQuotaReset(best, now)
	for _, candidate := range available[1:] {
		reset, known := weeklyQuotaReset(candidate, now)
		if known && (!bestKnown || reset.Before(bestReset)) {
			best, bestReset, bestKnown = candidate, reset, true
		}
	}
	return best, nil
}

// weeklyQuotaReset returns when the credential's longest observed quota window
// resets. ok is false when no window was observed or the observed one already reset.
func weeklyQuotaReset(auth *Auth, now time.Time) (reset time.Time, ok bool) {
	if auth == nil {
		return time.Time{}, false
	}
	var raw string
	switch strings.ToLower(strings.TrimSpace(auth.Provider)) {
	case "claude":
		raw = auth.Quota.Signals["Anthropic-Ratelimit-Unified-7d-Reset"]
	case "codex":
		raw = longestCodexWindowReset(auth.Quota.Signals)
	}
	unix, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || unix <= 0 {
		return time.Time{}, false
	}
	reset = time.Unix(unix, 0)
	return reset, reset.After(now)
}

// longestCodexWindowReset returns the reset-at value of the longest Codex window.
// Plans differ in which slot carries the weekly window: some report it as
// primary with an empty secondary, others as secondary next to a 5-hour primary.
func longestCodexWindowReset(signals map[string]string) string {
	var resetAt string
	longest := 0
	for _, slot := range []string{"Primary", "Secondary"} {
		minutes, err := strconv.Atoi(signals["X-Codex-"+slot+"-Window-Minutes"])
		if err == nil && minutes > longest {
			longest = minutes
			resetAt = signals["X-Codex-"+slot+"-Reset-At"]
		}
	}
	return resetAt
}
