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
	CURR_PLAYBACK_URL = "https://api.spotify.com/v1/me/player"
	RELAX_PLAYLIST	  = "spotify:playlist:0qPA1tBtiCLVHCUfREECnO"
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
	Device	 string `json:"device"`
	IsActive bool	`json:"is_active"`
}

// Home is the Sporify instance used in home
// Main is the main instance of Spotify that i use
type Environment string

const (
	HOME = "home"
	MAIN = "Home"
)

var EnvironmentName = map[Environment]string{
	HOME: "home",
	MAIN: "main",
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
	case HOME:
		envPrefix = "HOME_"
		sp.Devices = []string{"MacBook Air de Richard", "iPhone"}
	case MAIN:
		envPrefix = "MAIN_"
		sp.Devices = []string{"librespot"}
	default:
		return nil
	}

	sp.ClientId = os.Getenv(envPrefix + "SP_CLIENT_ID")
	sp.ClientSecret = os.Getenv(envPrefix + "SP_CLIENT_SECRET")
	sp.CallbackUri = os.Getenv(envPrefix + "SP_CALLBACK_URI")
	sp.tokensFilePath = fmt.Sprintf(".tokens/.tokens-%s.txt", string(environment))

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
				result.AccessToken = value
			} else if key == "refresh_token" {
				result.RefreshToken = value
			}
		}
	}

	if result.AccessToken == "" || result.RefreshToken == "" {
		return nil, errors.New(fmt.Sprintf("error retrieving data from file: %s", fileName))
	}

	return &result, nil
}

// Update the current active environment
func updateEnv(newEnv *Spotify) {
	currentEnv = newEnv
}

func getCurrentPlayBack(c *gin.Context) (*Playback, error) {
	accessToken, exists := c.Get("access_token")

	if !exists {
		defaultNotAccessTokenResponse(c)
		return nil, fmt.Errorf("error getting current playback")
	}

	playbackResponse := Playback{}
	resp, resp_err := makeRequest("GET", CURR_PLAYBACK_URL, accessToken.(string))

	if err := json.NewDecoder(resp.Body).Decode(&playbackResponse); resp_err != nil || err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Error retrieving current playback data",
		})
		return nil, err
	}

	return &playbackResponse, nil
}

func getDeviceId(deviceName string, accessToken string) (string, error) {
	urlStr := "https://api.spotify.com/v1/me/player/devices"

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

	return deviceId, nil
}

func schedule(epochMillis int, action func()) {
	seconds := epochMillis/1000 - int(time.Now().UnixMilli())/1000

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

	req, err := http.NewRequest(method, urlStr, bodyReader)

	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	client := &http.Client{}

	resp, err := client.Do(req)

	if err != nil {
		return nil, fmt.Errorf("failed in request: %w", err)
	}

	defer resp.Body.Close()

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

	jsonBody, err := json.Marshal(gin.H{
		"context_uri": contextUri,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	deviceId, _ := getDeviceId(deviceName, accessToken)
	setVolume(volumePercent, accessToken, deviceId)

	return makeRequest("PUT", urlStr, accessToken, jsonBody)
}

func pausePlayback(accessToken string, deviceName string) (*http.Response, error) {
	urlStr := "https://api.spotify.com/v1/me/player/pause"

	if deviceName == "" {
		deviceName = currentEnv.Devices[0]
	}

	return makeRequest("PUT", urlStr, accessToken)
}

func defaultNotAccessTokenResponse(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"error": "Access token not found",
	})
}

// Verify tokens and context
func SpotifyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if currentEnv == nil {
			currentEnv = new(Environment("home"))
		}

		tokens, err := readTokensFromFile(currentEnv.tokensFilePath)

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Spotify tokens not found",
			})
			c.Abort()
			return
		}

		c.Set("access_token", tokens.AccessToken)
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

	scopeList := []string{"user-read-playback-state", "user-modify-playback-state"}
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
	percentaje := c.Query("percentaje")

	volume, err := strconv.Atoi(percentaje)

	if percentaje == "" || err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Volume percentaje is required and must be correct",
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

	setVolume(volume, accessToken.(string), currentPlayback.Device)
	c.JSON(http.StatusOK, gin.H{
		"message": "Playback paused successfully",
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
				RELAX_PLAYLIST,
				accessToken.(string),
				60,
			)
		}
	case "sleep":
		fn = func() {
			pausePlayback(accessToken.(string), homeDevice)
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
	percentaje := c.Query("percentaje")

	if percentaje == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "percentaje is required",
		})
		return
	}

	volume, err := strconv.Atoi(percentaje)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "percentaje must be a number",
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

	setVolume(volume, accessToken.(string), currentPlayback.Device)
}
