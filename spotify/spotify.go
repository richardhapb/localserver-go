package spotify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var (
	currentEnv *Spotify
	envs	   = make(map[string]Spotify)
)

const (
	CurrentPlaybackEndpoint = "https://api.spotify.com/v1/me/player"
	RelaxPlaylistUri		= "spotify:playlist:0qPA1tBtiCLVHCUfREECnO"
)

type Spotify struct {
	CallbackUri    string
	ClientId	   string
	ClientSecret   string
	Devices		   []string
	tokensFilePath string
}

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type Playback struct {
	Device struct {
		ID	 string `json:"id"`
		Name string `json:"name"`
	} `json:"device"`
	IsActive bool `json:"is_active"`
}

// Home is the Sporify instance used in home
// Main is the main instance of Spotify that i use
type Environment string

const (
	Home = "home"
	Main = "main"
)

var EnvironmentName = map[Environment]string{
	Home: "home",
	Main: "main",
}

func new(environment Environment) *Spotify {
	// If exists return it, this avoid duplicates instances
	if _, exists := envs[string(environment)]; exists {
		sp := envs[string(environment)]
		log.Println("Returning existent Spotify instance")
		return &sp
	}
	log.Println("Creating new Spotify instance")

	if err := godotenv.Load(); err != nil {
		log.Println(".env not found")
		return nil
	}

	var sp Spotify
	envPrefix := ""

	switch environment {
	case Main:
		envPrefix = "MAIN_"
		sp.Devices = []string{"iPhone", "MacBook Air de Richard"}
	case Home:
		envPrefix = "HOME_"
		sp.Devices = []string{"librespot"}
	default:
		return nil
	}

	sp.ClientId = os.Getenv(envPrefix + "SP_CLIENT_ID")
	sp.ClientSecret = os.Getenv(envPrefix + "SP_CLIENT_SECRET")
	sp.CallbackUri = os.Getenv(envPrefix + "SP_CALLBACK_URI")
	sp.tokensFilePath = fmt.Sprintf(".tokens/.tokens-%s.txt", string(environment))

	envs[string(environment)] = sp
	return &sp
}

// Write tokens to a file for storage them
func writeTokensToFile(tokensLines *Tokens, fileName string) error {
	dir := strings.Split(fileName, "/")
	dirName := strings.Join(dir[:len(dir)-1], "/")
	_, err := os.Stat(dirName)

	if err != nil && os.IsNotExist(err) {
		os.MkdirAll(dirName, os.ModePerm)
	}

	log.Println(fmt.Sprintf("Writing tokens to file %s", fileName))

	tokens := []string{
		"access_token:" + tokensLines.AccessToken,
		"refresh_token:" + tokensLines.RefreshToken,
	}

	data := []byte(strings.Join(tokens, "\n") + "\n")
	return os.WriteFile(fileName, data, 0600)
}

func readTokensFromFile(fileName string) (*Tokens, error) {
	data, err := os.ReadFile(fileName)
	result := Tokens{}

	log.Println(fmt.Sprintf("Reading tokens from file %s", fileName))

	if err != nil {
		return nil, err
	}

	dataStr := string(data)
	tokens := strings.Split(dataStr, "\n")

	for _, token := range tokens {
		elements := strings.SplitN(token, ":", 2)

		if len(elements) == 2 {
			key := strings.TrimSpace(elements[0])
			value := strings.TrimSpace(elements[1])

			if key == "access_token" {
				log.Println("access token found")
				result.AccessToken = value
			} else if key == "refresh_token" {
				log.Println("refresh token found")
				result.RefreshToken = value
			}
			if result.AccessToken != "" && result.RefreshToken != "" {
				break
			}
		}
	}

	if result.RefreshToken == "" {
		return nil, errors.New(fmt.Sprintf("error retrieving data from file: %s", fileName))
	}

	return &result, nil
}

// Update the current active environment
func updateEnv(newEnv *Spotify) {
	log.Println(fmt.Sprintf("Settings environment to %s", newEnv.Devices))
	currentEnv = newEnv
}

func getCurrentPlayBack(c *gin.Context) (*Playback, error) {
	log.Println("Getting current playback")
	accessToken, exists := c.Get("access_token")
	if !exists {
		return nil, errors.New("no access token available")
	}

	resp, err := makeRequest("GET", CurrentPlaybackEndpoint, accessToken.(string))
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	// Handle 204 No Content - means no active playback
	if resp.StatusCode == http.StatusNoContent {
		return &Playback{IsActive: false}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var playback Playback
	if err := json.NewDecoder(resp.Body).Decode(&playback); err != nil {
		return nil, fmt.Errorf("decoding playback response: %w", err)
	}

	log.Printf("Playback found: %+v", playback)
	return &playback, nil
}

func getDeviceId(deviceName string, accessToken string) (string, error) {
	urlStr := "https://api.spotify.com/v1/me/player/devices"

	log.Println(fmt.Sprintf("Retrieving id for device %s", deviceName))

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return "", fmt.Errorf("Failed to execute request: %w", err)
	}

	defer resp.Body.Close()

	var devicesResponse struct {
		Devices []struct {
			ID	 string `json:"id"`
			Name string `json:"name"`
		} `json:"devices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&devicesResponse); err != nil {
		return "", fmt.Errorf("Failed in request when retrieving device: %w", err)
	}

	deviceId := ""

	for _, device := range devicesResponse.Devices {
		if device.Name == deviceName {
			deviceId = device.ID
		}

		if deviceId != "" {
			break
		}
	}

	log.Println(fmt.Sprintf("Device id found: %s", deviceId))

	return deviceId, nil
}

func schedule(epochMillis int, action func()) {
	seconds := epochMillis/1000 - int(time.Now().UnixMilli())/1000

	log.Println(fmt.Sprintf("Scheduling task to %i seconds later", seconds))

	if seconds < 0 {
		log.Println("epochMillis is in the past in schedule function")
		return
	}

	go func() {
		time.Sleep(time.Duration(seconds) * time.Second)
		action()
	}()
}

func makeRequest(method string, urlStr string, accessToken string, body ...[]byte) (*http.Response, error) {
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewBuffer(body[0])
	}

	log.Println(fmt.Sprintf("Making request to %s", urlStr))
	req, err := http.NewRequest(method, urlStr, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed in request: %w", err)
	}

	log.Println(fmt.Sprintf("Request status: %s", resp.Status))
	return resp, nil
}

func setVolume(volumePercent int, accessToken string, deviceName string) (*http.Response, error) {
	baseUrl := "https://api.spotify.com/v1/me/player/volume"
	deviceId, _ := getDeviceId(deviceName, accessToken)

	params := url.Values{}
	params.Set("volume_percent", strconv.Itoa(volumePercent))
	params.Set("device_id", deviceId)

	urlStr := baseUrl + "?" + params.Encode()

	return makeRequest("PUT", urlStr, accessToken)
}

func playPlaylist(deviceName string, contextUri string, accessToken string, volumePercent int) (*http.Response, error) {
	urlStr := "https://api.spotify.com/v1/me/player/play"

	log.Println(fmt.Sprintf("Playing list with URI %s", contextUri))

	jsonBody, err := json.Marshal(gin.H{
		"context_uri": contextUri,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	setVolume(volumePercent, accessToken, deviceName)
	enableShuffle(accessToken, deviceName)
	enableRepeat(accessToken, deviceName)

	return makeRequest("PUT", urlStr, accessToken, jsonBody)
}

func pausePlayback(accessToken string, deviceName ...string) (*http.Response, error) {
	urlStr := "https://api.spotify.com/v1/me/player/pause"
	deviceN := ""
	deviceId := ""

	if len(deviceName) == 0 {
		deviceN = envs[Home].Devices[0]
	}

	deviceId, _ = getDeviceId(deviceN, accessToken)

	jsonBody, err := json.Marshal(gin.H{
		"device_id": deviceId,
	})

	if err != nil {
		log.Println("Error setting the device to pause; continuing with default settings.")
	}

	return makeRequest("PUT", urlStr, accessToken, jsonBody)
}

func defaultNotAccessTokenResponse(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"error": "Access token not found",
	})
}

func getEnvFromDeviceName(deviceName string) *Spotify {
	if deviceName == "" {
		return nil
	}

	// Loop through environments checking device lists
	for _, env := range envs {
		for _, device := range env.Devices {
			if device == deviceName {
				log.Println("Device name found, returning instance")
				return &env
			}
		}
	}

	return nil
}

func refreshToken(refreshToken string, sp *Spotify) (string, error) {

	// Use url.Values for proper form encoding
	data := url.Values{
		"grant_type":	 {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":	 {sp.ClientId},
		"client_secret": {sp.ClientSecret},
	}

	req, err := http.NewRequest(
		"POST",
		"https://accounts.spotify.com/api/token",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("bad response (%d): %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType	string `json:"token_type"`
		ExpiresIn	int    `json:"expires_in"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}

	// Update file with new tokens
	if err := writeTokensToFile(&Tokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: refreshToken,
	}, sp.tokensFilePath); err != nil {
		return "", fmt.Errorf("writing tokens: %w", err)
	}

	return tokenResp.AccessToken, nil
}

func enableShuffle(accessToken string, deviceName string) {
	baseUrl := "https://api.spotify.com/v1/me/player/shuffle"

	deviceId, err := getDeviceId(deviceName, accessToken)

	params := url.Values{}
	params.Set("state", "true")
	if err == nil && deviceId != "" {
		params.Set("device_id", deviceId)
	}

	urlStr := baseUrl + "?" + params.Encode()

	makeRequest("PUT", urlStr, accessToken)
}

func enableRepeat(accessToken string, deviceName string) {
	baseUrl := "https://api.spotify.com/v1/me/player/repeat"

	deviceId, err := getDeviceId(deviceName, accessToken)

	params := url.Values{}
	params.Set("state", "context")
	if err == nil && deviceId != "" {
		params.Set("device_id", deviceId)
	}

	urlStr := baseUrl + "?" + params.Encode()

	makeRequest("PUT", urlStr, accessToken)
}

// Verify tokens and context
func SpotifyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		// Initialize
		if len(envs[Home].Devices) == 0 || len(envs[Main].Devices) == 0 {
			envs[Home] = *new(Home)
			envs[Main] = *new(Main)
		}

		reqEnv := c.Query("env")
		deviceName, _ := url.QueryUnescape(c.Query("device_name"))

		if reqEnv != "" {
			log.Printf("Retrieving data from env: %s", reqEnv)
			sp := envs[reqEnv]
			currentEnv = &sp
		} else if deviceName != "" {
			log.Printf("Retrieving data for device name: %s\n", deviceName)
			currentEnv = getEnvFromDeviceName(deviceName)
		}

		if currentEnv == nil || currentEnv.tokensFilePath == "" {
			currentEnv = new(Environment(Home))
		}

		tokens, err := readTokensFromFile(currentEnv.tokensFilePath)

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": fmt.Sprintf("Spotify tokens not found: %s", err),
			})
			c.Abort()
			return
		}

		accessToken, err := refreshToken(tokens.RefreshToken, currentEnv)

		if err != nil {
			log.Printf("Error refreshing token, setting from file: %w\n", err)
			accessToken = tokens.AccessToken
		}

		c.Set("access_token", accessToken)
		c.Set("refresh_token", tokens.RefreshToken)
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

	if err := writeTokensToFile(&tokenResponse, sp.tokensFilePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save tokens: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Login ready!",
	})
}

func Pause(c *gin.Context) {
	device_name := c.Query("device_name")

	if device_name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "device_name is required",
		})
		return
	}

	accessToken, exists := c.Get("access_token")

	if !exists {
		defaultNotAccessTokenResponse(c)
		return
	}

	if _, err := getCurrentPlayBack(c); err == nil {
		_, err := pausePlayback(accessToken.(string))
		if err == nil {
			c.JSON(http.StatusOK, gin.H{
				"message": "Music paused successfully",
			})
			return
		}
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

	accessToken, exists := c.Get("access_token")

	if !exists {
		defaultNotAccessTokenResponse(c)
		return
	}

	var fn func()
	homeDevice := envs["home"].Devices[0]

	switch action {
	case "alarm":
		fn = func() {
			playPlaylist(
				homeDevice,
				RelaxPlaylistUri,
				accessToken.(string),
				60,
			)
		}
	case "sleep":
		fn = func() {
			pausePlayback(accessToken.(string))
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
	deviceName := c.DefaultQuery("device_name", currentEnv.Devices[0])

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

	accessToken, exists := c.Get("access_token")

	if !exists {
		defaultNotAccessTokenResponse(c)
		return
	}

	resp, err := playPlaylist(deviceName, uri, accessToken.(string), volume)
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

	accessToken, exists := c.Get("access_token")

	if !exists {
		defaultNotAccessTokenResponse(c)
		return
	}

	currentPlayback, err := getCurrentPlayBack(c)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "There is not a playback currently",
		})
		return
	}

	setVolume(volume, accessToken.(string), currentPlayback.Device.Name)

	c.JSON(http.StatusOK, gin.H{
		"message": "Volume setted successfully",
	})
}
