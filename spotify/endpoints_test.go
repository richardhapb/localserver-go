package spotify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFetchDevicesNilTokens(t *testing.T) {
	sp := &Spotify{Name: "home"}
	if _, err := sp.fetchDevices(); err == nil {
		t.Fatal("fetchDevices() with nil tokens = nil error, want error")
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
