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
	envs       = make(map[string]Spotify)
)

const (
	CurrentPlaybackEndpoint = "https://api.spotify.com/v1/me/player"
	UserQueueEndpoint       = "https://api.spotify.com/v1/me/player/queue"
	RelaxPlaylistUri        = "spotify:playlist:0qPA1tBtiCLVHCUfREECnO"
)

type Spotify struct {
	CallbackUri    string
	ClientId       string
	ClientSecret   string
	Devices        []string
	tokensFilePath string
}

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type Playback struct {
	Device struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		IsActive bool   `json:"is_active"`
	} `json:"device"`
	RepeatState  string `json:"repeat_state"`
	ShuffleState bool   `json:"shuffle_state"`
	Context      struct {
		Type string `json:"type"`
		Uri  string `json:"uri"`
	} `json:"context"`
	ProgressMs int  `json:"progress_ms"`
	IsPlaying  bool `json:"is_playing"`
}

type Track struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Uri  string `json:"uri"`
}

type UserQueue struct {
	CurrentlyPlaying Track `json:"currently_playing"`
	Queue []Track `json:"queue"`
}

// Home is the Spotify instance used in home
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
		log.Fatalln(".env not found")
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
		return &Playback{}, nil
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
			ID   string `json:"id"`
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

	log.Println(fmt.Sprintf("Scheduling task to %d seconds later", seconds))

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

	defer setVolume(volumePercent, accessToken, deviceName)
	defer enableShuffle(accessToken, deviceName)
	defer enableRepeat(accessToken, deviceName)

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
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {sp.ClientId},
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
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
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

