package spotify

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Verify tokens and context
func SpotifyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		// Initialize
		if envs[Home] == nil {
			envs[Home] = new(Home)
		}
		if envs[Main] == nil {
			envs[Main] = new(Main)
		}

		reqEnv := c.Query("env")
		deviceName, _ := url.QueryUnescape(c.Query("device_name"))
		from, _ := url.QueryUnescape(c.Query("from"))

		if reqEnv != "" {
			log.Printf("Retrieving data from env: %s", reqEnv)
			updateEnv(envs[reqEnv])
		} else if deviceName != "" {
			if env := getEnvFromDeviceName(deviceName); env != nil {
				updateEnv(env)
			}
		} else if from != "" {
			if env := getEnvFromDeviceName(from); env != nil {
				updateEnv(env)
			}
		} else {
			// Home as default
			updateEnv(envs[Home])
		}

		if _, err := currentEnv.refreshToken(); err != nil {
			log.Printf("Error refreshing token, setting from file: %s\n", err)
		}

		c.Next()
	}
}

// Handle the login in Spotify using Client ID and Client Secret
func Login(c *gin.Context) {
	errMsg := "Account is incorrect. You need to pass the account type as a URL argument: env={account type}. It should be either home or main."

	environment := c.Query("env")

	if environment == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errMsg,
		})
		return
	}

	if _, exists := EnvironmentName[Environment(environment)]; !exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": errMsg,
		})
		return
	}

	sp := new(Environment(environment))
	updateEnv(sp)

	scopeList := []string{
		"user-read-playback-state",
		"user-modify-playback-state",
		"user-read-currently-playing",
		"app-remote-control",
		"user-read-recently-played",
	}
	scope := strings.Join(scopeList, " ")

	params := url.Values{}
	params.Set("client_id", sp.ClientId)
	params.Set("response_type", "code")
	params.Set("redirect_uri", sp.CallbackUri)
	params.Set("scope", scope)

	authUrl := "https://accounts.spotify.com/authorize?" + params.Encode()

	c.Redirect(http.StatusTemporaryRedirect, authUrl)
}

// Handle the Spotify callback when login
func Callback(c *gin.Context) {

	sp := currentEnv

	if sp == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "missing spotify account type (home or main)",
		})
		return
	}

	code := c.Query("code")

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "missing callback code",
		})
		return
	}

	values := url.Values{}
	values.Add("grant_type", "authorization_code")
	values.Add("code", code)
	values.Add("redirect_uri", sp.CallbackUri)
	values.Add("client_id", sp.ClientId)
	values.Add("client_secret", sp.ClientSecret)

	tokenUrl := "https://accounts.spotify.com/api/token"

	resp, err := http.Post(tokenUrl, "application/x-www-form-urlencoded", strings.NewReader(values.Encode()))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to request token: " + err.Error()})
		return
	}

	defer resp.Body.Close()

	tokenResponse := Tokens{}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse token response: " + err.Error()})
		return
	}

	currentEnv.tokens = &tokenResponse

	if err := writeTokensToFile(&tokenResponse, sp.tokensFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save tokens: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Login ready!",
	})
}

// resolveTargetDevice finds the requested device in sp's live device list. On
// failure it writes the appropriate error response and returns ok=false, so the
// caller can just `return`.
func resolveTargetDevice(c *gin.Context, sp *Spotify, deviceName string) (*Device, bool) {
	device, err := sp.deviceByName(deviceName)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("could not reach Spotify to resolve device: %v", err),
		})
		return nil, false
	}
	if device == nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"error": fmt.Sprintf("device %q is not currently reachable; open the Spotify app on it and retry", deviceName),
		})
		return nil, false
	}
	return device, true
}

// playbackError reports a non-2xx Spotify playback response as an error. The
// body must not have been consumed yet.
func playbackError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	return fmt.Errorf("spotify returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

func Play(c *gin.Context) {
	deviceName := c.Query("device_name")

	if deviceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "device_name is required",
		})
		return
	}

	sp := getEnvFromDeviceName(deviceName)
	if sp == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown device_name: %s", deviceName)})
		return
	}

	device, ok := resolveTargetDevice(c, sp, deviceName)
	if !ok {
		return
	}

	resp, err := sp.playPlayback(device.ID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to play playback: %v", err)})
		return
	}
	defer resp.Body.Close()

	if err := playbackError(resp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to play playback: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Music playing successfully",
		"device_name": deviceName,
	})
}

func Pause(c *gin.Context) {
	deviceName := c.Query("device_name")

	if deviceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "device_name is required",
		})
		return
	}

	sp := getEnvFromDeviceName(deviceName)
	if sp == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown device_name: %s", deviceName)})
		return
	}

	device, ok := resolveTargetDevice(c, sp, deviceName)
	if !ok {
		return
	}

	resp, err := sp.pausePlayback(device.ID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to pause playback: %v", err)})
		return
	}
	defer resp.Body.Close()

	if err := playbackError(resp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to pause playback: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Music paused successfully",
		"device_name": deviceName,
	})
}

func Schedule(c *gin.Context) {
	action := c.Query("action")
	timeMillis := c.Query("time_millis")

	if timeMillis == "" || action == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "action and time_millis are required",
		})
		return
	}

	var fn func()

	switch action {
	case "alarm":
		fn = func() {
			device, err := currentEnv.activeDevice()
			if err != nil {
				log.Printf("alarm: could not resolve device: %v", err)
			}
			currentEnv.playPlaylist(device, RelaxPlaylistUri, 60)
		}
	case "sleep":
		fn = func() {
			currentEnv.pausePlayback("")
		}
	}

	epochMillis, err := strconv.Atoi(timeMillis)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pub_milis must be an integer",
		})
		return
	}

	schedule(int64(epochMillis), fn)
	c.JSON(http.StatusOK, gin.H{
		"message": "Schedule setted successfully",
	})
}

func PlayPlaylist(c *gin.Context) {
	uri := c.Query("uri")
	volumeStr := c.DefaultQuery("volume", "80")
	deviceName := c.DefaultQuery("device_name", currentEnv.Devices[0].Name)
	sp := getEnvFromDeviceName(deviceName)

	if uri == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "URI is required",
		})
		return
	}

	if sp == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown device_name: %s", deviceName)})
		return
	}

	volume, err := strconv.Atoi(volumeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid volume value",
		})
		return
	}

	device, ok := resolveTargetDevice(c, sp, deviceName)
	if !ok {
		return
	}

	resp, err := sp.playPlaylist(device, uri, volume)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("Error playing playlist: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	if err := playbackError(resp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("Error playing playlist: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Playlist started successfully",
		"uri":         uri,
		"device_name": deviceName,
		"volume":      volume,
	})
}

func SearchAndPlayPlaylist(c *gin.Context) {
	query := c.Query("query")
	volumeStr := c.DefaultQuery("volume", "40")
	deviceName := c.DefaultQuery("device_name", currentEnv.Devices[0].Name)
	sp := getEnvFromDeviceName(deviceName)

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "query is required",
		})
		return
	}

	if sp == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("unknown device_name: %s", deviceName),
		})
		return
	}

	volume, err := strconv.Atoi(volumeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid volume value",
		})
		return
	}

	device, ok := resolveTargetDevice(c, sp, deviceName)
	if !ok {
		return
	}

	uri, playlistName, err := sp.searchPlaylist(query)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("Error searching playlist: %v", err),
		})
		return
	}

	resp, err := sp.playPlaylist(device, uri, volume)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("Error playing playlist: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	if err := playbackError(resp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":    fmt.Sprintf("Spotify playback failed: %v", err),
			"playlist": playlistName,
			"uri":      uri,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "Playlist started successfully",
		"playlist":    playlistName,
		"uri":         uri,
		"device_name": deviceName,
		"volume":      volume,
	})
}

func Volume(c *gin.Context) {
	percentage := c.Query("percentage")

	if percentage == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "percentage is required",
		})
		return
	}

	volume, err := strconv.Atoi(percentage)

	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "percentage must be a number",
		})
		return
	}

	device, err := currentEnv.activeDevice()
	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("could not reach Spotify to resolve device: %v", err),
		})
		return
	}
	if device == nil {
		c.JSON(http.StatusFailedDependency, gin.H{
			"error": "no reachable device to set volume on",
		})
		return
	}

	resp, err := currentEnv.setVolume(device.ID, volume, device.SupportsVolume)

	if err != nil {
		log.Println(err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	if err := playbackError(resp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to set volume: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Volume setted successfully",
	})
}

// Devices returns every reachable device grouped by environment. It refreshes
// each environment's token and queries Spotify for all available devices,
// regardless of whether one is actively playing.
func Devices(c *gin.Context) {
	type envDevices struct {
		Environment string   `json:"environment"`
		Devices     []Device `json:"devices"`
	}

	environments := make([]envDevices, 0, len(envs))

	for name, env := range envs {
		if env == nil {
			continue
		}

		if _, err := env.refreshToken(); err != nil {
			log.Printf("Devices: failed to refresh token for %s: %s", name, err)
		}

		devices, err := env.fetchDevices()
		if err != nil {
			log.Printf("Devices: failed to fetch devices for %s: %s", name, err)
		}
		if devices == nil {
			devices = []Device{}
		}

		environments = append(environments, envDevices{
			Environment: name,
			Devices:     devices,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"environments": environments,
	})
}

func TransferPlayback(c *gin.Context) {
	toName := c.Query("to")
	volumeStr := c.DefaultQuery("volume", "0")

	volume, err := strconv.Atoi(volumeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid volume value",
		})
		return
	}

	if toName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "the to parameter is required",
		})
		return
	}

	from := currentEnv
	to := getEnvFromDeviceName(toName)

	if from == nil || to == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid devices: to=%s", toName),
		})
		return
	}

	if _, err := to.refreshToken(); err != nil {
		log.Printf("Error refreshing token, setting from file: %s\n", err)
	}

	toDevice, ok := resolveTargetDevice(c, to, toName)
	if !ok {
		return
	}

	// Librespot does not allow playing a queue directly.
	// For it, i need to transfer the current song and schedule the playlist.
	if toName == "librespot" || toName == "iPhone" {
		err = from.hardTransferPlayback(to, toDevice, volume)
	} else {
		// TODO: Evaluate whether this is necessary; if not, remove it.
		err = from.transferPlayback(to, toDevice)
	}

	if err != nil {
		log.Printf("Error transferring callback: %s", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("error transfering playback: %s", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Playback transferred successfully",
	})
}
