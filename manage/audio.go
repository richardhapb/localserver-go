//go:build linux && !nogpio

package manage

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/warthog618/go-gpiocdev"
)

const (
	LAudio = 24
	RAudio = 25
)

type audioController struct {
	lAudio *gpiocdev.Line
	rAudio *gpiocdev.Line
}

var audioControl audioController

func (ac *audioController) switchToDevice(deviceName string) error {
	value := audioValue(deviceName)

	var lErr, rErr error
	if err := ac.lAudio.SetValue(value); err != nil {
		lErr = fmt.Errorf("failed to set pin %d: %w", LAudio, err)
	}
	if err := ac.rAudio.SetValue(value); err != nil {
		rErr = fmt.Errorf("failed to set pin %d: %w", RAudio, err)
	}

	return errors.Join(lErr, rErr)
}

func InitializeAudio() error {
	var err error
	audioControl.lAudio, err = gpiocdev.RequestLine("gpiochip0", LAudio, gpiocdev.AsOutput(0))
	if err != nil {
		return fmt.Errorf("GPIO initialization failed for pin %d: %s", LAudio, err)
	}

	audioControl.rAudio, err = gpiocdev.RequestLine("gpiochip0", RAudio, gpiocdev.AsOutput(0))
	if err != nil {
		return fmt.Errorf("GPIO initialization failed for pin %d: %s", RAudio, err)
	}

	return nil
}

func SwitchAudio(c *gin.Context) {
	if audioControl.lAudio == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("pin %d is not bound", LAudio)})
		return
	}
	if audioControl.rAudio == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("pin %d is not bound", RAudio)})
		return
	}

	deviceName := c.Query("device_name")

	if deviceName == "" {
		value, err := audioControl.lAudio.Value()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error querying pins"})
			return
		}

		target := otherDevice(value)
		if err := audioControl.switchToDevice(target); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "Audio swapped successfully"})
		return
	}

	if !isValidDevice(deviceName) {
		c.JSON(http.StatusOK, gin.H{"status": fmt.Sprintf("Invalid devices, supported: %v", ValidDevices)})
		return
	}

	if err := audioControl.switchToDevice(deviceName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": fmt.Sprintf("Audio switched to %s", deviceName)})
}
