package spotify

import (
	"encoding/json"
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
