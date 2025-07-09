package manage

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type devicesResponse struct {
	Devices []struct {
		Addresses  []string `json:"addresses"`
		Name       string   `json:"name"`
		ID         string   `json:"id"`
		NodeID     string   `json:"nodeId"`
		Hostname   string   `json:"hostname"`
		OS         string   `json:"os"`
		LastSeen   string   `json:"lastSeen"`
		IsExternal bool     `json:"isExternal"`
	} `json:"devices"`
}

type deviceAttributes struct {
	name          string
	macEnv        string
	wakeCommands  []string
	sleepCommands []string
	battCommands  []string
}

type deviceData struct {
	username   string
	name       string
	ip         string
	mac        string
	attritutes *deviceAttributes
}

type jnAttributes struct {
	Category      string `json:"category"`
	Time          string `json:"time"`
	Description   string `json:"description"`
	Notification  string `json:"notification"`
	UnlimitedTime bool   `json:"unlimited"`
	Headless      bool   `json:"headless"`
}

type Config struct {
	OpenAIKey string `envconfig:"OPENAI_API_KEY" required:"true"`
	TSApiKey  string `envconfig:"TS_API_KEY" required:"true"`
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

func newDevicesAttributes() *[]deviceAttributes {
	var da []deviceAttributes

	da = append(da, deviceAttributes{
		name:   "macbook",
		macEnv: "MAC_MAC",
		wakeCommands: []string{
			"caffeinate -u -t 1",
		},
		sleepCommands: []string{
			"pmset sleepnow",
		},
		battCommands: []string{"pmset -g batt | grep -o '[0-9]\\+%' | sed 's/%//' "},
	})

	da = append(da, deviceAttributes{
		name:   "arch-richard",
		macEnv: "MAC_ARCH",
		wakeCommands: []string{
			"DISPLAY=:0 xset dpms 0 0 600",
			"DISPLAY=:0 xset dpms force on",
		},
		sleepCommands: []string{
			"DISPLAY=:0 xset dpms 0 0 5",
			"DISPLAY=:0 i3lock -n -c 000000 >/dev/null 2>&1 &",
		},
		battCommands: []string{"cat /sys/class/power_supply/BAT1/capacity"},
	})

	return &da
}

func buildJnCommand(args jnAttributes) []string {
	cmd := []string{"-d"}
	if args.Headless {
		cmd = append(cmd, "-H")
	}
	cmd = append(cmd, "-t", args.Time, "-c", args.Category)

	if args.UnlimitedTime {
		cmd = append(cmd, "-u")
	}
	if args.Description != "" {
		cmd = append(cmd, "-l", args.Description)
	}
	if args.Notification != "" {
		cmd = append(cmd, "-n", args.Notification)
	}

	return cmd
}

func getDeviceAtt(name string) *deviceAttributes {
	devices := newDevicesAttributes()

	for _, device := range *devices {
		if device.name == name {
			return &device
		}
	}

	return nil
}

func validateRequest(c *gin.Context) (*deviceData, error) {
	device := deviceData{}
	device.name = c.Query("name")

	if device.name == "" {
		return nil, fmt.Errorf("name is required")
	}

	device.attritutes = getDeviceAtt(device.name)

	if device.attritutes == nil {
		return nil, fmt.Errorf("device not found")
	}

	urlStr := "https://api.tailscale.com/api/v2/tailnet/richardhapb.github/devices"
	mac := os.Getenv(device.attritutes.macEnv)

	if cfg.TSApiKey == "" || mac == "" {
		return nil, fmt.Errorf("Api key or MAC not found")
	}

	device.mac = mac

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating request: %s\n", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.TSApiKey))

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("Error in request: %s\n", err)
	}

	defer resp.Body.Close()

	var devices devicesResponse

	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		return nil, fmt.Errorf("Error decoding response: %s\n", err)
	}

	log.Printf("Devices: %v", devices)

	ip := captureDeviceIP(device.name, &devices)

	if ip == "" {
		return nil, fmt.Errorf("Device not found: %s", device.name)
	}

	device.ip = ip
	device.username = "richard"

	return &device, nil
}

func Wake(c *gin.Context) {

	device, err := validateRequest(c)

	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err})
		return
	}

	_, err = executeCommands(device, device.attritutes.wakeCommands)
	if err != nil {
		log.Printf("Command failed: %s\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Command failed: %s", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Command executed successfully"})
}

func Sleep(c *gin.Context) {

	device, err := validateRequest(c)

	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err})
		return
	}

	if err := sendWOL(device.mac); err != nil {
		log.Println(err)
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("WOL failed: %s", err)})
		return
	}

	_, err = executeCommands(device, device.attritutes.sleepCommands)
	if err != nil {
		log.Printf("Command failed: %s\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Command failed: %s", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Command executed successfully"})
}

func Battery(c *gin.Context) {
	device, err := validateRequest(c)

	if err != nil {
		log.Printf("Command failed: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Command failed: %s", err)})
		return
	}

	batt, err := executeCommands(device, device.attritutes.battCommands)
	if err != nil {
		log.Printf("Command failed: %s\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Command failed: %s", err)})
		return
	}

	log.Printf("Battery of %s: %s", device.name, batt)
	c.JSON(http.StatusOK, gin.H{"battery": batt})
}

func LaunchJn(c *gin.Context) {
	var jnRequest jnAttributes
	if err := c.ShouldBindJSON(&jnRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid request: %v", err)})
		return
	}

	jnPath := getJNPath()

	cmd := exec.Command(jnPath, buildJnCommand(jnRequest)...)
	logFile, err := os.Create("/tmp/jn.log")
	if err != nil {
		log.Printf("Failed to create log file: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create log file"})
		return
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Start the process without waiting
	if err := cmd.Start(); err != nil {
		log.Printf("Failed to start jn: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start Just-Notify"})
		return
	}

	// Detach the process
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("jn process ended with error: %v", err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Just-Notify started successfully"})
}

func TermSignalJn(c *gin.Context) {
	category := c.Query("category")

	if category == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "category is required"})
		return
	}

	jnPath := getJNPath()

	// Kill jn process
	if err := exec.Command(jnPath, "-k", "-c", category).Run(); err != nil {
		log.Printf("Failed to terminate jn: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to terminate jn process: %s", err)})
		return
	}

	// Give process time to write final log and read it
	time.Sleep(500 * time.Millisecond)
	output, err := os.ReadFile("/tmp/jn.log")
	if err != nil {
		log.Printf("Failed to read jn log: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to read termination status: %s", err)})
		return
	}

	lines := strings.Split(string(output), "\n")
	timeElapsed := ""
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "elapsed") {
			idx := strings.Index(lines[i], "Time elapsed")
			if idx != -1 {
				timeElapsed = lines[i][idx:]
			}
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Just-Notify terminated: %s", timeElapsed)})
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



func executeCommands(device *deviceData, commands []string) (string, error) {
	if err := sendWOL(device.mac); err != nil {
		log.Println(err)
		return "", fmt.Errorf("WOL failed: %s", err)
	}

	var lastResponse string
	for _, cmd := range commands {
		response, err := sendCommand(cmd, device.username, device.ip)
		if err != nil {
			log.Printf("Command failed: %s", err)
			return "", fmt.Errorf("Command failed: %s", err)
		}
		lastResponse = response
	}

	return lastResponse, nil
}

func captureDeviceIP(name string, devices *devicesResponse) string {
	for _, device := range devices.Devices {
		log.Printf("Checking device: %s", device.Hostname)
		if device.Hostname == name {
			if len(device.Addresses) > 0 && len(device.Addresses[0]) > 0 {
				ip := device.Addresses[0]
				log.Printf("Found IP address for %s: %s", name, ip)
				return ip
			}
			log.Printf("No valid IP address found for device %s", name)
		}
	}
	log.Printf("Device %s not found", name)
	return ""
}

func sendCommand(command, user, host string) (string, error) {

	cmd := exec.Command("ssh", user+"@"+host, command)

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("SSH command failed: %v\nOutput: %s", err, string(output))
	}

	log.Printf("Command '%s' executed successfully on %s@%s", command, user, host)

	return strings.TrimSpace(string(output)), nil
}

func sendWOL(mac string) error {
	// Get the MAC address for the target machine from ARP table or configuration
	cmd := exec.Command("wakeonlan", mac)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wake-on-lan command failed: %v\nOutput: %s", err, string(output))
	}

	log.Printf("Wake-on-LAN packet sent to %s: %s", mac, string(output))
	return nil
}

func getJNPath() string {
	// TODO: make this dynamic
	jnPath := filepath.Join(os.Getenv("HOME"), ".local", "bin", "jn")
	if _, err := os.Stat(jnPath); err != nil {
		log.Fatal("jn executable not found. Please ensure Just-Notify is installed and in PATH")
	}

	return jnPath
}
