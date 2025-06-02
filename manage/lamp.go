//go:build gpio
// +build gpio

package manage

import (
	"fmt"
	"github.com/warthog618/go-gpiocdev"
	"github.com/gin-gonic/gin"
	"net/http"
)


var lampControl struct {
	line *gpiocdev.Line
	on   bool
}


func InitializeLamp() error {
	var err error
	lampControl.line, err = gpiocdev.RequestLine("gpiochip0", 17, gpiocdev.AsOutput(0))

	if err != nil {
		return fmt.Errorf("GPIO initialization failed: %s", err)
	}

	return nil
}

func ToggleLamp(c *gin.Context) {
	if lampControl.line == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Raspberry Pi pin 17 is not bound,",
		})
		return
	}

	if lampControl.on {
		lampControl.line.SetValue(0)
		lampControl.on = false
		c.JSON(http.StatusOK, gin.H{
			"status": "Lamp off",
		})
		return
	}

	lampControl.line.SetValue(1)
	lampControl.on = true
	c.JSON(http.StatusOK, gin.H{
		"status": "Lamp on",
	})
}


