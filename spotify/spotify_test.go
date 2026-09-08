package spotify

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShouldRandomizeOffset(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		total      int
		want       bool
	}{
		{"ok with tracks", http.StatusOK, 12, true},
		{"ok with empty playlist", http.StatusOK, 0, false},
		{"not found", http.StatusNotFound, 12, false},
		{"not found with zero total", http.StatusNotFound, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRandomizeOffset(tt.statusCode, tt.total); got != tt.want {
				t.Errorf("shouldRandomizeOffset(%d, %d) = %v, want %v", tt.statusCode, tt.total, got, tt.want)
			}
		})
	}
}

func TestIsRetryablePlayStatus(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{http.StatusNotFound, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusOK, false},
		{http.StatusNoContent, false},
		{http.StatusForbidden, false},
		{http.StatusUnauthorized, false},
	}

	for _, tt := range tests {
		if got := isRetryablePlayStatus(tt.status); got != tt.want {
			t.Errorf("isRetryablePlayStatus(%d) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

// withNoRetryDelay zeroes the backoff for the duration of a test.
func withNoRetryDelay(t *testing.T) {
	t.Helper()
	saved := playRetryDelay
	playRetryDelay = 0
	t.Cleanup(func() { playRetryDelay = saved })
}

// The core of the fix: a transient 404 right after the device wakes (e.g. the
// alarm cron hitting Spotify before it has finished registering the device as
// active) must be retried instead of surfaced as a failed alarm.
func TestPutWithRetryRecoversFromTransientNotFound(t *testing.T) {
	withNoRetryDelay(t)

	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	sp := &Spotify{tokens: &Tokens{AccessToken: "test"}}
	resp, err := sp.putWithRetry(ts.URL)
	if err != nil {
		t.Fatalf("putWithRetry() error = %v", err)
	}
	defer resp.Body.Close()

	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

// A device that is genuinely gone must not retry forever -- it gives up after
// maxPlayAttempts and hands the caller the last response.
func TestPutWithRetryGivesUpAfterMaxAttempts(t *testing.T) {
	withNoRetryDelay(t)

	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	sp := &Spotify{tokens: &Tokens{AccessToken: "test"}}
	resp, err := sp.putWithRetry(ts.URL)
	if err != nil {
		t.Fatalf("putWithRetry() error = %v", err)
	}
	defer resp.Body.Close()

	if calls != maxPlayAttempts {
		t.Errorf("calls = %d, want %d", calls, maxPlayAttempts)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// A non-transient failure (e.g. a dead grant) must fail fast, not burn
// through retries.
func TestPutWithRetryDoesNotRetryNonTransientStatus(t *testing.T) {
	withNoRetryDelay(t)

	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	sp := &Spotify{tokens: &Tokens{AccessToken: "test"}}
	resp, err := sp.putWithRetry(ts.URL)
	if err != nil {
		t.Fatalf("putWithRetry() error = %v", err)
	}
	defer resp.Body.Close()

	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

// Sanity check that the configured backoff actually elapses between retries.
func TestPutWithRetryWaitsBetweenAttempts(t *testing.T) {
	saved := playRetryDelay
	playRetryDelay = 20 * time.Millisecond
	defer func() { playRetryDelay = saved }()

	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	sp := &Spotify{tokens: &Tokens{AccessToken: "test"}}
	start := time.Now()
	resp, err := sp.putWithRetry(ts.URL)
	if err != nil {
		t.Fatalf("putWithRetry() error = %v", err)
	}
	defer resp.Body.Close()

	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Errorf("elapsed = %s, want at least the configured backoff", elapsed)
	}
}
