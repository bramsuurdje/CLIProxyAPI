package auth

import (
	"context"
	"strconv"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
)

func claudeAuthResettingIn(id string, in time.Duration) *Auth {
	return &Auth{
		ID:       id,
		Provider: "claude",
		Quota: QuotaState{Signals: map[string]string{
			"Anthropic-Ratelimit-Unified-7d-Reset": strconv.FormatInt(time.Now().Add(in).Unix(), 10),
		}},
	}
}

func TestResetFirstSelectorPick_PrefersSoonestWeeklyReset(t *testing.T) {
	t.Parallel()

	auths := []*Auth{
		claudeAuthResettingIn("a", 5*24*time.Hour),
		{ID: "b", Provider: "claude"}, // never used: no running window
		claudeAuthResettingIn("c", 2*time.Hour),
		claudeAuthResettingIn("d", -time.Hour), // window already rolled over
	}

	got, err := (&ResetFirstSelector{}).Pick(context.Background(), "claude", "", cliproxyexecutor.Options{}, auths)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if got.ID != "c" {
		t.Fatalf("Pick() auth.ID = %q, want %q", got.ID, "c")
	}
}

func TestResetFirstSelectorPick_WithoutSignalsBehavesLikeFillFirst(t *testing.T) {
	t.Parallel()

	auths := []*Auth{{ID: "b"}, {ID: "a"}, {ID: "c"}}

	got, err := (&ResetFirstSelector{}).Pick(context.Background(), "gemini", "", cliproxyexecutor.Options{}, auths)
	if err != nil {
		t.Fatalf("Pick() error = %v", err)
	}
	if got.ID != "a" {
		t.Fatalf("Pick() auth.ID = %q, want %q", got.ID, "a")
	}
}

func TestLongestCodexWindowReset_UsesLongestSlot(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		signals map[string]string
		want    string
	}{
		"weekly in primary, empty secondary": {
			signals: map[string]string{
				"X-Codex-Primary-Window-Minutes":   "10080",
				"X-Codex-Primary-Reset-At":         "200",
				"X-Codex-Secondary-Window-Minutes": "0",
			},
			want: "200",
		},
		"5-hour primary, weekly secondary": {
			signals: map[string]string{
				"X-Codex-Primary-Window-Minutes":   "300",
				"X-Codex-Primary-Reset-At":         "100",
				"X-Codex-Secondary-Window-Minutes": "10080",
				"X-Codex-Secondary-Reset-At":       "200",
			},
			want: "200",
		},
		"no windows": {signals: nil, want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := longestCodexWindowReset(tt.signals); got != tt.want {
				t.Fatalf("longestCodexWindowReset() = %q, want %q", got, tt.want)
			}
		})
	}
}
