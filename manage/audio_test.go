package manage

import "testing"

func TestIsValidDevice(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		want       bool
	}{
		{name: "rpi lowercase", deviceName: "rpi", want: true},
		{name: "monitor lowercase", deviceName: "monitor", want: true},
		{name: "mixed case", deviceName: "Monitor", want: true},
		{name: "unknown device", deviceName: "tv", want: false},
		{name: "empty string", deviceName: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidDevice(tt.deviceName); got != tt.want {
				t.Errorf("isValidDevice(%q) = %v, want %v", tt.deviceName, got, tt.want)
			}
		})
	}
}

func TestAudioValue(t *testing.T) {
	tests := []struct {
		name       string
		deviceName string
		want       int
	}{
		{name: "rpi is 0", deviceName: "rpi", want: 0},
		{name: "rpi mixed case is 0", deviceName: "RPI", want: 0},
		{name: "monitor is 1", deviceName: "monitor", want: 1},
		{name: "unknown device defaults to 1", deviceName: "tv", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := audioValue(tt.deviceName); got != tt.want {
				t.Errorf("audioValue(%q) = %d, want %d", tt.deviceName, got, tt.want)
			}
		})
	}
}

func TestOtherDevice(t *testing.T) {
	tests := []struct {
		name         string
		currentValue int
		want         string
	}{
		{name: "from rpi to monitor", currentValue: 0, want: "monitor"},
		{name: "from monitor to rpi", currentValue: 1, want: "rpi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := otherDevice(tt.currentValue); got != tt.want {
				t.Errorf("otherDevice(%d) = %q, want %q", tt.currentValue, got, tt.want)
			}
		})
	}
}
