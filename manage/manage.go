package manage

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
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

func WakeArch(c *gin.Context) {

	if err := godotenv.Load(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": ".env file not found"})
		return
	}

	urlStr := "https://api.tailscale.com/api/v2/tailnet/richardhapb.github/devices"
	apiKey := os.Getenv("TS_API_KEY")
	mac := os.Getenv("MAC_ARCH")

	if apiKey == "" || mac == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Api key or MAC not found"})
		return
	}
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Error creating request: %s\n", err)})
		return
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Error in request: %s\n", err)})
		return
	}

	defer resp.Body.Close()

	var devices devicesResponse

	if err := json.NewDecoder(resp.Body).Decode(&devices); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("Error decoding response: %s\n", err)})
		return
	}

	log.Printf("Devices: %v", devices)

	deviceName := "arch-richard"
	ip := captureDeviceIP(deviceName, &devices)

	if ip == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Device not found: %s", deviceName)})
		return
	}


	if err := sendWOL(mac); err != nil {
		log.Fatalln(err)
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("WOL failed: %s", err)})
		return
	}

	if err = sendCommand("wake", "richard", ip); err != nil {
		log.Fatalln(err)
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Command failed: %s", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Command executed successfully"})
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

func sendCommand(command, user, host string) error {

	cmd := exec.Command("ssh", "-v", user+"@"+host, command)
	
	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SSH command failed: %v\nOutput: %s", err, string(output))
	}

	log.Printf("Command '%s' executed successfully on %s@%s", command, user, host)
	return nil
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

