package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFetchDevicesNilTokens(t *testing.T) {
	sp := &Spotify{Name: "home"}
	if _, err := sp.fetchDevices(); err == nil {
		t.Fatal("fetchDevices() with nil tokens = nil error, want error")
	}
}

func TestDeviceByNameNilTokens(t *testing.T) {
	sp := &Spotify{Name: "main"}
	if _, err := sp.deviceByName("MacBook Air de Richard"); err == nil {
		t.Fatal("deviceByName() with nil tokens = nil error, want error")
	}
}

// The core of the bug fix: playback URLs must carry the requested device_id so
// Spotify targets it instead of falling back to some "active" default.
func TestAppendDeviceID(t *testing.T) {
	const play = "https://api.spotify.com/v1/me/player/play"

	got := appendDeviceID(play, "a81906ace2720092304129d29ecfd0831b2d26b5")
	want := play + "?device_id=a81906ace2720092304129d29ecfd0831b2d26b5"
	if got != want {
		t.Errorf("appendDeviceID() = %q, want %q", got, want)
	}

	// Empty device id must leave the URL untouched (Spotify default target).
	if got := appendDeviceID(play, ""); got != play {
		t.Errorf("appendDeviceID(empty) = %q, want %q", got, play)
	}

	// Existing query params must be preserved.
	base := "https://api.spotify.com/v1/me/player/volume?volume_percent=40"
	got = appendDeviceID(base, "dev123")
	if !strings.Contains(got, "volume_percent=40") || !strings.Contains(got, "device_id=dev123") {
		t.Errorf("appendDeviceID() lost a param: %q", got)
	}
}

// The other half: handlers must not report success on a non-2xx Spotify reply.
func TestPlaybackError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{name: "204 no content is success", status: http.StatusNoContent, body: "", wantErr: false},
		{name: "200 ok is success", status: http.StatusOK, body: "", wantErr: false},
		{name: "404 no active device", status: http.StatusNotFound, body: `{"error":{"reason":"NO_ACTIVE_DEVICE"}}`, wantErr: true},
		{name: "403 forbidden", status: http.StatusForbidden, body: "nope", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.status,
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			err := playbackError(resp)
			if (err != nil) != tt.wantErr {
				t.Fatalf("playbackError() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.body) && tt.body != "" {
				t.Errorf("playbackError() = %v, want it to include body %q", err, tt.body)
			}
		})
	}
}

// A play request with no device_name must fail fast, not silently target a
// wrong device.
func TestPlayRequiresDeviceName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/spotify/play", nil)

	Play(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// A rejected token must never decode into "no devices reachable" -- that is what
// made a revoked grant look like an offline speaker.
func TestParseDevicesResponse(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantDevices int
		wantErr     bool
		wantReauth  bool
	}{
		{
			name:        "200 with devices",
			status:      http.StatusOK,
			body:        `{"devices":[{"id":"abc","name":"librespot","is_active":true}]}`,
			wantDevices: 1,
		},
		{
			name:   "200 with no devices",
			status: http.StatusOK,
			body:   `{"devices":[]}`,
		},
		{
			name:       "401 expired token",
			status:     http.StatusUnauthorized,
			body:       `{"error":{"status":401,"message":"The access token expired"}}`,
			wantErr:    true,
			wantReauth: true,
		},
		{
			name:       "403 insufficient scope",
			status:     http.StatusForbidden,
			body:       `{"error":{"status":403,"message":"Insufficient client scope"}}`,
			wantErr:    true,
			wantReauth: true,
		},
		{
			name:    "500 from Spotify is not a login problem",
			status:  http.StatusInternalServerError,
			body:    "boom",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices, err := parseDevicesResponse(tt.status, []byte(tt.body))

			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDevicesResponse() err = %v, wantErr %v", err, tt.wantErr)
			}
			if got := errors.Is(err, ErrReauthRequired); got != tt.wantReauth {
				t.Errorf("errors.Is(err, ErrReauthRequired) = %v, want %v (err = %v)", got, tt.wantReauth, err)
			}
			if len(devices) != tt.wantDevices {
				t.Errorf("devices = %d, want %d", len(devices), tt.wantDevices)
			}
		})
	}
}

// Spotify may rotate the refresh token; reusing the superseded one is what gets
// the whole grant revoked.
func TestParseTokenRefresh(t *testing.T) {
	current := &Tokens{AccessToken: "old-access", RefreshToken: "old-refresh"}

	t.Run("keeps the stored refresh token when none is returned", func(t *testing.T) {
		got, err := parseTokenRefresh(http.StatusOK, []byte(`{"access_token":"new-access","expires_in":3600}`), current)
		if err != nil {
			t.Fatalf("parseTokenRefresh() err = %v", err)
		}
		if got.AccessToken != "new-access" {
			t.Errorf("access token = %q, want %q", got.AccessToken, "new-access")
		}
		if got.RefreshToken != "old-refresh" {
			t.Errorf("refresh token = %q, want %q", got.RefreshToken, "old-refresh")
		}
	})

	t.Run("stores a rotated refresh token", func(t *testing.T) {
		got, err := parseTokenRefresh(http.StatusOK, []byte(`{"access_token":"new-access","refresh_token":"new-refresh"}`), current)
		if err != nil {
			t.Fatalf("parseTokenRefresh() err = %v", err)
		}
		if got.RefreshToken != "new-refresh" {
			t.Errorf("refresh token = %q, want %q", got.RefreshToken, "new-refresh")
		}
	})

	t.Run("revoked refresh token needs a new login", func(t *testing.T) {
		_, err := parseTokenRefresh(
			http.StatusBadRequest,
			[]byte(`{"error":"invalid_grant","error_description":"Refresh token revoked"}`),
			current,
		)
		if !errors.Is(err, ErrReauthRequired) {
			t.Fatalf("err = %v, want ErrReauthRequired", err)
		}
		if !strings.Contains(err.Error(), "Refresh token revoked") {
			t.Errorf("err = %v, want it to include Spotify's description", err)
		}
	})

	t.Run("other failures are not login problems", func(t *testing.T) {
		_, err := parseTokenRefresh(http.StatusInternalServerError, []byte("boom"), current)
		if err == nil {
			t.Fatal("err = nil, want error")
		}
		if errors.Is(err, ErrReauthRequired) {
			t.Errorf("err = %v, want a plain error", err)
		}
	})

	t.Run("success without an access token is an error", func(t *testing.T) {
		if _, err := parseTokenRefresh(http.StatusOK, []byte(`{"expires_in":3600}`), current); err == nil {
			t.Fatal("err = nil, want error")
		}
	})
}

// A dead grant must answer 401 with the login URL, not 424 "device unreachable".
func TestRespondDeviceLookupError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sp := &Spotify{Name: "home"}

	t.Run("reauth required", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		respondDeviceLookupError(c, sp, fmt.Errorf("%w: token rejected", ErrReauthRequired))

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}

		var body struct {
			Error string `json:"error"`
			Fix   string `json:"fix"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if body.Fix != "/spotify/login?env=home" {
			t.Errorf("fix = %q, want %q", body.Fix, "/spotify/login?env=home")
		}
	})

	t.Run("spotify unreachable", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)

		respondDeviceLookupError(c, sp, errors.New("dial tcp: no route to host"))

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
		}
	})
}

func TestDevicesEndpointEmptyEnvs(t *testing.T) {
	// Isolate the package-global env map and restore it afterwards.
	saved := envs
	envs = make(map[string]*Spotify)
	defer func() { envs = saved }()

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/spotify/devices", nil)

	Devices(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Environments []struct {
			Environment string   `json:"environment"`
			Devices     []Device `json:"devices"`
		} `json:"environments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if len(body.Environments) != 0 {
		t.Errorf("environments = %v, want empty", body.Environments)
	}
}
