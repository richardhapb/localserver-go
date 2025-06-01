package manage

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/warthog618/go-gpiocdev"
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
}

var lamp struct {
	line *gpiocdev.Line
	on   bool
}

func InitializeLamp() error {
	var err error
	lamp.line, err = gpiocdev.RequestLine("gpiochip0", 17, gpiocdev.AsOutput(0))

	if err != nil {
		return err
	}

	return nil
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

func (dd *deviceData) buildJnCommand(args jnAttributes) []string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("jn -t %s -c %s", args.Time, args.Category))

	if args.UnlimitedTime {
		sb.WriteString(" -u")
	}
	if args.Description != "" {
		fmt.Fprintf(&sb, " -l %q", args.Description)
	}
	if args.Notification != "" {
		fmt.Fprintf(&sb, " -n %q", args.Notification)
	}

	sb.WriteString(" > /tmp/jn.log 2>&1 &")
	
	return []string{sb.String()}
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

	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf(".env file not found")
	}

	urlStr := "https://api.tailscale.com/api/v2/tailnet/richardhapb.github/devices"
	apiKey := os.Getenv("TS_API_KEY")
	mac := os.Getenv(device.attritutes.macEnv)

	if apiKey == "" || mac == "" {
		return nil, fmt.Errorf("Api key or MAC not found")
	}

	device.mac = mac

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating request: %s\n", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

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

func ToggleLamp(c *gin.Context) {
	if lamp.line == nil {
		c.JSON(http.StatusOK, gin.H{
			"error": "Raspberry Pi pin 17 is not bound,",
		})
		return
	}

	if lamp.on {
		lamp.line.SetValue(0)
		lamp.on = false
		c.JSON(http.StatusOK, gin.H{
			"status": "Lamp off",
		})
		return
	}

	lamp.line.SetValue(1)
	lamp.on = true
	c.JSON(http.StatusOK, gin.H{
		"status": "Lamp on",
	})
}

func LaunchJn(c *gin.Context) {
	device, err := validateRequest(c)

	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err})
		return
	}

	var jnRequest jnAttributes

	if err := json.NewDecoder(c.Request.Body).Decode(&jnRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid attributes: %s", err)})
	}

	_, err = executeCommands(device, device.buildJnCommand(jnRequest))
	if err != nil {
		log.Printf("Command failed: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Command failed: %s", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Just-Notify executed successfully"})
}

func TermSignalJn(c *gin.Context) {
	device, err := validateRequest(c)

	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err})
		return
	}

	result, err := executeCommands(device, []string{"pkill -SIGTERM jn && sleep 0.1 && tail -1 /tmp/jn.log"})
	if err != nil {
		log.Printf("Command failed: %s", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Command failed: %s", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("SIGNTERM signal sent to Just-Notify: %s", result)})
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
