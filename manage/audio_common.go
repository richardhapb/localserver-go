package manage

import (
	"slices"
	"strings"
)

var ValidDevices = []string{"rpi", "monitor"}

func isValidDevice(deviceName string) bool {
	return slices.Contains(ValidDevices, strings.ToLower(deviceName))
}

// audioValue is the GPIO line value that routes audio to deviceName: rpi
// (onboard, normally closed) is 0, monitor is 1.
func audioValue(deviceName string) int {
	if strings.ToLower(deviceName) == ValidDevices[0] {
		return 0
	}
	return 1
}

// otherDevice returns the device opposite the line's current value, used
// when SwitchAudio is called without an explicit device_name.
func otherDevice(currentValue int) string {
	if currentValue == 0 {
		return ValidDevices[1]
	}
	return ValidDevices[0]
}
