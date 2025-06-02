//go:build !gpio
// +build !gpio

package manage

import "github.com/gin-gonic/gin"


var lampControl struct {
    on bool
}

func InitializeLamp() error {
    return nil
}

func ToggleLamp(c *gin.Context) {
    lampControl.on = !lampControl.on
    c.JSON(200, gin.H{
        "status": map[bool]string{false: "Lamp off", true: "Lamp on"}[lampControl.on] + " (dev mode)",
    })
}
