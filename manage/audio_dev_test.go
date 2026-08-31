//go:build !linux || nogpio

package manage

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func switchAudioRequest(t *testing.T, query string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/manage/audio/switch"+query, nil)

	SwitchAudio(c)
	return rec
}

func TestSwitchAudioValidDevice(t *testing.T) {
	saved := audioControl
	defer func() { audioControl = saved }()
	audioControl.value = 0

	rec := switchAudioRequest(t, "?device_name=monitor")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if audioControl.value != 1 {
		t.Errorf("audioControl.value = %d, want 1", audioControl.value)
	}
}

func TestSwitchAudioInvalidDevice(t *testing.T) {
	saved := audioControl
	defer func() { audioControl = saved }()
	audioControl.value = 0

	rec := switchAudioRequest(t, "?device_name=tv")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if audioControl.value != 0 {
		t.Errorf("audioControl.value = %d, want unchanged 0", audioControl.value)
	}
}

func TestSwitchAudioSwapTogglesDevice(t *testing.T) {
	saved := audioControl
	defer func() { audioControl = saved }()
	audioControl.value = 0

	rec := switchAudioRequest(t, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if audioControl.value != 1 {
		t.Fatalf("after first swap audioControl.value = %d, want 1", audioControl.value)
	}

	rec = switchAudioRequest(t, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if audioControl.value != 0 {
		t.Errorf("after second swap audioControl.value = %d, want 0", audioControl.value)
	}
}
