package manage

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const LAMP_ENDPOINT = "http://192.168.1.50/toggle-lamp"

var lampClient = &http.Client{Timeout: 3 * time.Second}

func ToggleLamp(c *gin.Context) {
	req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, LAMP_ENDPOINT, nil)
	resp, err := lampClient.Do(req)
	if err != nil {
		// The relay toggles on receipt even when the ESP is slow to answer.
		// Return 202 so the client treats it as done and doesn't retry → no double toggle.
		c.JSON(http.StatusAccepted, gin.H{"msg": "toggle sent (device did not confirm)"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	c.JSON(http.StatusOK, gin.H{"msg": string(body)})
}
