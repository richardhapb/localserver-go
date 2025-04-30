package spotify

import (
	"testing"
	"time"
)

func TestSchedule(t *testing.T) {
	const deltaMilli = 100

	tests := []struct {
		name     string
		schedule int64
		want     bool
	}{
		{
			name:     "future schedule",
			schedule: time.Now().UnixMilli() + deltaMilli,
			want:     true,
		},
		{
			name:     "past schedule",
			schedule: time.Now().UnixMilli() - deltaMilli,
			want:     false,
		},
	}

	for _, tt := range tests {
		tt := tt // Capture range variable
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan bool, 1)

			schedule(tt.schedule, func() {
				done <- true
			})

			select {
			case <-done:
				if !tt.want {
					t.Errorf("%s: schedule executed when it should not have", tt.name)
				}
			case <-time.After(deltaMilli * 2 * time.Millisecond):
				if tt.want {
					t.Errorf("%s: schedule did not execute when it should have", tt.name)
				}
			}
		})
	}
}

func TestParsePlaylistId(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "valid playlist URI",
			input:   "spotify:playlist:0qPA1tBtiCLVHCUfREECnO",
			want:    "0qPA1tBtiCLVHCUfREECnO",
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "spotify:invalid:0qPA1tBtiCLVHCUfREECnO",
			want:    "",
			wantErr: true,
		},
		{
			name:    "Extra section ",
			input:   "spotify:playlist:0qPA1tBtiCLVHCUfREECnO:another",
			want:    "",
			wantErr: true,
		},
		{
			name:    "too few parts",
			input:   "spotify:playlist",
			want:    "",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePlaylistId(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePlaylistId() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parsePlaylistId() = %v, want %v", got, tt.want)
			}
		})
	}
}
