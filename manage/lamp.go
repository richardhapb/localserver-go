package manage

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

const LAMP_ENDPOINT = "http://192.168.1.50/toggle-lamp"

func ToggleLamp(c *gin.Context) {
	resp, err := http.Get(LAMP_ENDPOINT)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Error toggling lamp: %s", err),
		})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Error reading response: %s", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"msg": string(body),
	})
}
