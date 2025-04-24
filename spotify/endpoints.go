package spotify

import (
	"encoding/json"
	"fmt"
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

func Play(c *gin.Context) {
	deviceName := c.Query("device_name")

	if deviceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "device_name is required",
		})
		return
	}

	sp := getEnvFromDeviceName(deviceName)

	_, err := sp.playPlayback()
	if err == nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "Music playing successfully",
		})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "failed to play playback"})
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

	_, err := sp.pausePlayback()
	if err == nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "Music paused successfully",
		})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "failed to pause playback"})
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
			currentEnv.playPlaylist(
				RelaxPlaylistUri,
				60,
			)
		}
	case "sleep":
		fn = func() {
			currentEnv.pausePlayback()
		}
	}

	epochMillis, err := strconv.Atoi(timeMillis)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "pub_milis must be an integer",
		})
		return
	}

	schedule(epochMillis, fn)
	c.JSON(http.StatusOK, gin.H{
		"message": "Schedule setted successfully",
	})
}

func Playlist(c *gin.Context) {
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

	volume, err := strconv.Atoi(volumeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid volume value",
		})
		return
	}

	resp, err := sp.playPlaylist(uri, volume)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Error playing playlist: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	c.JSON(http.StatusOK, gin.H{
		"message": "Playlist started successfully",
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
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "percentage must be a number",
		})
		return
	}

	currentEnv.setVolume(volume)

	c.JSON(http.StatusOK, gin.H{
		"message": "Volume setted successfully",
	})
}

func TransferPlayback(c *gin.Context) {
	fromName := c.Query("from")
	toName := c.Query("to")

	if fromName == "" || toName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "from and to are required",
		})
		return
	}

	from := getEnvFromDeviceName(fromName)
	to := getEnvFromDeviceName(toName)

	if from == nil || to == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid devices: %s, %s", fromName, toName),
		})
		return
	}

	var err error

	// Librespot does not allow playing a queue directly.
	// For it, i need to transfer the current song and schedule the playlist.
	if toName == "librespot" || toName == "iPhone" {
		err = from.hardTransferPlayback(to)
	} else {
		// TODO: Evaluate whether this is necessary; if not, remove it.
		err = from.transferPlayback(to)
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
