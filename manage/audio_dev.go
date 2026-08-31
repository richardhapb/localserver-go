//go:build !linux || nogpio

package manage

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

var audioControl struct {
	value int
}

func InitializeAudio() error {
	return nil
}

func SwitchAudio(c *gin.Context) {
	deviceName := c.Query("device_name")

	if deviceName == "" {
		target := otherDevice(audioControl.value)
		audioControl.value = audioValue(target)
		c.JSON(http.StatusOK, gin.H{"status": "Audio swapped successfully (dev mode)"})
		return
	}

	if !isValidDevice(deviceName) {
		c.JSON(http.StatusOK, gin.H{"status": fmt.Sprintf("Invalid devices, supported: %v", ValidDevices)})
		return
	}

	audioControl.value = audioValue(deviceName)
	c.JSON(http.StatusOK, gin.H{"status": fmt.Sprintf("Audio switched to %s (dev mode)", deviceName)})
}
