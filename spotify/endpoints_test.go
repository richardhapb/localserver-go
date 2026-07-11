package spotify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFilterActiveDevices(t *testing.T) {
	tests := []struct {
		name  string
		input []Device
		want  []Device
	}{
		{
			name:  "nil slice returns empty",
			input: nil,
			want:  []Device{},
		},
		{
			name: "no active devices",
			input: []Device{
				{Name: "iPhone", IsActive: false},
				{Name: "MacBook", IsActive: false},
			},
			want: []Device{},
		},
		{
			name: "filters and preserves order",
			input: []Device{
				{Name: "iPhone", IsActive: false},
				{Name: "librespot", IsActive: true},
				{Name: "MacBook", IsActive: false},
				{Name: "Speaker", IsActive: true},
			},
			want: []Device{
				{Name: "librespot", IsActive: true},
				{Name: "Speaker", IsActive: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterActiveDevices(tt.input)
			if got == nil {
				t.Fatal("filterActiveDevices() returned nil, want non-nil slice")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterActiveDevices() = %v, want %v", got, tt.want)
			}
		})
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
