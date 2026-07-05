package manage

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	OpenAIKey string `envconfig:"OPENAI_API_KEY" required:"true"`
}

// Global config instance
var cfg Config

func init() {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}
	if err := envconfig.Process("", &cfg); err != nil {
		log.Fatalf("Failed to process environment config: %v", err)
	}
}

func ReviewGrammar(c *gin.Context) {
	var content struct {
		Text string `json:"text" binding:"required"`
	}

	if err := c.ShouldBindJSON(&content); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: missing or invalid text field"})
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to determine user home directory"})
		return
	}

	neospellerPath := filepath.Join(home, ".local", "bin", "neospeller")
	if _, err := os.Stat(neospellerPath); os.IsNotExist(err) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Neospeller binary not found"})
		return
	}

	cmd := exec.Command(neospellerPath, "--lang", "text")
	cmd.Env = append(os.Environ(), fmt.Sprintf("OPENAI_API_KEY=%s", cfg.OpenAIKey))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create input pipe"})
		return
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, content.Text)
	}()

	out, err := cmd.CombinedOutput()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Neospeller execution failed: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"corrections": string(out)})
}
