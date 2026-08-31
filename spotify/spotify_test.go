package spotify

import (
	"net/http"
	"testing"
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
