package spotify

import (
	"bytes"
	"encoding/json"
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
	envs       = make(map[string]*Spotify)
	debugMode  = os.Getenv("DEBUG") == "true"
)

const (
	CurrentPlaybackEndpoint = "https://api.spotify.com/v1/me/player"
	UserQueueEndpoint       = "https://api.spotify.com/v1/me/player/queue"
	PlayEndpoint            = "https://api.spotify.com/v1/me/player/play"
	RelaxPlaylistUri        = "spotify:playlist:0qPA1tBtiCLVHCUfREECnO"
)

type Spotify struct {
	Name           string
	CallbackUri    string
	ClientId       string
	ClientSecret   string
	Devices        []Device
	tokensFilePath string
	tokens         *Tokens
}

type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type Playback struct {
	Device       Device `json:"device"`
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
	CurrentlyPlaying Track   `json:"currently_playing"`
	Queue            []Track `json:"queue"`
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
		return sp
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
		sp.Devices = []Device{{"", "iPhone", false}, {"", "MacBook Air de Richard", false}}
	case Home:
		envPrefix = "HOME_"
		sp.Devices = []Device{{"", "librespot", false}}
	default:
		return nil
	}

	sp.Name = string(environment)
	sp.ClientId = os.Getenv(envPrefix + "SP_CLIENT_ID")
	sp.ClientSecret = os.Getenv(envPrefix + "SP_CLIENT_SECRET")
	sp.CallbackUri = os.Getenv(envPrefix + "SP_CALLBACK_URI")
	sp.tokensFilePath = fmt.Sprintf(".tokens/.tokens-%s.txt", string(environment))

	if tokens, err := readTokensFromFile(sp.tokensFilePath); err == nil {
		sp.tokens = tokens
	} else {
		log.Printf("tokens not found for %s", sp.Name)
	}

	envs[string(environment)] = &sp
	return &sp
}

func (sp *Spotify) String() string {

	names := make([]string, 0, len(sp.Devices))

	for _, device := range sp.Devices {
		names = append(names, device.Name)
	}

	return fmt.Sprintf("Name: %s, Devices: %v", sp.Name, strings.Join(names, ", "))
}

func (sp *Spotify) getActiveDevice() *Device {
	for _, device := range sp.Devices {
		if device.IsActive {
			return &device
		}
	}

	// Return the first one as the default
	return &sp.Devices[0]
}

func (sp *Spotify) getActiveDeviceName() string {
	if device := sp.getActiveDevice(); device != nil {
		return device.Name
	}

	return ""
}

func (sp *Spotify) getActiveDeviceId() string {
	if device := sp.getActiveDevice(); device != nil {
		if device.ID != "" {
			return device.ID
		}

		deviceId, err := sp.getDeviceId(sp.getActiveDeviceName())
		if err == nil {
			device.ID = deviceId
			return deviceId
		}
		log.Printf("Error getting device ID: %v", err)
	}

	return ""
}

func (sp *Spotify) getDeviceId(deviceName string) (string, error) {
	urlStr := "https://api.spotify.com/v1/me/player/devices"

	if deviceName == "" {
		return "", fmt.Errorf("device name is empty")
	}

	log.Println(fmt.Sprintf("Retrieving id for device %s", deviceName))

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sp.tokens.AccessToken))

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return "", fmt.Errorf("Failed to execute request: %w", err)
	}

	defer resp.Body.Close()

	var devicesResponse struct {
		Devices []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			IsActive bool   `json:"is_active"`
		} `json:"devices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&devicesResponse); err != nil {
		return "", fmt.Errorf("Failed in request when retrieving device: %w", err)
	}

	deviceId := ""
	log.Printf("Devices found: %v", devicesResponse.Devices)

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

func (sp *Spotify) makeRequest(method string, urlStr string, body ...[]byte) (*http.Response, error) {
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", sp.tokens.AccessToken))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed in request: %w", err)
	}

	log.Println(fmt.Sprintf("Request status: %s", resp.Status))
	return resp, nil
}

func (sp *Spotify) setVolume(volumePercent int) (*http.Response, error) {
	baseUrl := "https://api.spotify.com/v1/me/player/volume"
	deviceId, _ := sp.getDeviceId(sp.getActiveDeviceName())

	params := url.Values{}
	params.Set("volume_percent", strconv.Itoa(volumePercent))
	params.Set("device_id", deviceId)

	urlStr := baseUrl + "?" + params.Encode()

	return sp.makeRequest("PUT", urlStr)
}

func (sp *Spotify) playPlaylist(contextUri string, volumePercent int) (*http.Response, error) {

	log.Println(fmt.Sprintf("Playing list with URI %s", contextUri))

	jsonBody, err := json.Marshal(gin.H{
		"context_uri": contextUri,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	urlStr := sp.appendDeviceId(PlayEndpoint)

	defer sp.setVolume(volumePercent)
	defer sp.enableShuffle()
	defer sp.enableRepeat()

	return sp.makeRequest("PUT", urlStr, jsonBody)
}

func (sp *Spotify) pausePlayback() (*http.Response, error) {
	urlStr := "https://api.spotify.com/v1/me/player/pause"
	deviceId := ""

	deviceId = sp.getActiveDeviceId()

	jsonBody, err := json.Marshal(gin.H{
		"device_id": deviceId,
	})

	if err != nil {
		log.Println("Error setting the device to pause; continuing with default settings.")
	}

	return sp.makeRequest("PUT", urlStr, jsonBody)
}

func getEnvFromDeviceName(deviceName string) *Spotify {
	if deviceName == "" {
		return nil
	}

	log.Printf("Retrieving data for device name: %s\n", deviceName)

	// Loop through environments checking device lists
	for _, env := range envs {
		for _, device := range env.Devices {
			if device.Name == deviceName {
				log.Println("Device name found, returning instance")
				return env
			}
		}
	}

	return nil
}

func (sp *Spotify) refreshToken() (string, error) {

	// Use url.Values for proper form encoding
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {sp.tokens.RefreshToken},
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

	sp.tokens.AccessToken = tokenResp.AccessToken

	// Update file with new tokens
	if err := writeTokensToFile(&Tokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: sp.tokens.RefreshToken,
	}, sp.tokensFilePath); err != nil {
		return "", fmt.Errorf("writing tokens: %w", err)
	}

	return tokenResp.AccessToken, nil
}

func (sp *Spotify) enableShuffle() {
	baseUrl := "https://api.spotify.com/v1/me/player/shuffle"
	urlStr := sp.appendDeviceId(baseUrl)

	sp.makeRequest("PUT", urlStr)
}

func (sp *Spotify) enableRepeat() {
	baseUrl := "https://api.spotify.com/v1/me/player/repeat"

	urlStr := sp.appendDeviceId(baseUrl)

	sp.makeRequest("PUT", urlStr)
}

func (sp *Spotify) getCurrentPlayback() (*Playback, error) {
	log.Println("Getting current playback")

	resp, err := sp.makeRequest("GET", CurrentPlaybackEndpoint)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	// Handle 204 No Content - means no active playback
	if resp.StatusCode == http.StatusNoContent {
		return &Playback{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		printResponseBody(resp)
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var playback Playback
	if err := json.NewDecoder(resp.Body).Decode(&playback); err != nil {
		return nil, fmt.Errorf("decoding playback response: %w", err)
	}

	log.Printf("Playback found: %+v", playback)
	return &playback, nil
}

func (sp *Spotify) getUserQueue() (*UserQueue, error) {
	log.Println("Getting user queue")

	resp, err := sp.makeRequest("GET", UserQueueEndpoint)
	if err != nil {
		return nil, fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	// Handle 204 No Content - means no active playback
	if resp.StatusCode == http.StatusNoContent {
		return &UserQueue{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		printResponseBody(resp)
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var userQueue UserQueue
	if err := json.NewDecoder(resp.Body).Decode(&userQueue); err != nil {
		return nil, fmt.Errorf("decoding playback response: %w", err)
	}

	log.Printf("User queue found: %+v", userQueue)
	return &userQueue, nil
}

// Migrate callback from one account to anoter
func (sp *Spotify) transferPlayback(to *Spotify) error {
	if to == nil {
		return fmt.Errorf("destination Spotify instance is nil")
	}

	if _, err := to.refreshToken(); err != nil {
		return fmt.Errorf("failed to refresh destination token: %w", err)
	}

	playback, err := sp.getCurrentPlayback()
	if err != nil {
		return fmt.Errorf("failed to get current playback: %w", err)
	}

	userQueue, err := sp.getUserQueue()
	if err != nil {
		return fmt.Errorf("failed to get user queue: %w", err)
	}

	if len(userQueue.Queue) == 0 || !playback.IsPlaying {
		fmt.Println("There are no items in the Playing/Queue")
		return nil
	}

	log.Println("Making list of Uris")

	// Queue + current track
	uris := make([]string, 0, len(userQueue.Queue)+1)

	// Only add currently playing if it has a URI
	if userQueue.CurrentlyPlaying.Uri != "" {
		uris = append(uris, userQueue.CurrentlyPlaying.Uri)
	}

	if len(uris) == 0 {
		return fmt.Errorf("no valid URIs found to transfer")
	}

	for _, track := range userQueue.Queue {
		if track.Uri != "" {
			uris = append(uris, track.Uri)
		}
	}

	log.Printf("Uris: %v\n", uris)

	// First pause current playback
	if err := sp.pauseCurrentPlayback(); err != nil {
		return fmt.Errorf("failed to pause current playback: %w", err)
	}

	log.Println("Playing on another device...")
	if _, err = to.playUris(uris, playback.ProgressMs); err != nil {
		return err
	}

	return nil
}

func (sp *Spotify) playUris(uris []string, positionMs int) (*http.Response, error) {
	if len(uris) == 0 {
		return nil, fmt.Errorf("no URIs provided")
	}

	data := gin.H{
		"uris":        uris,
		"position_ms": positionMs,
	}

	jsonBody, err := json.Marshal(data)

	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	log.Printf("Attempting to play %d track(s) at position %d ms", len(uris), positionMs)

	urlStr := sp.appendDeviceId(PlayEndpoint)

	resp, err := sp.makeRequest("PUT", urlStr, jsonBody)

	if err != nil {
		return nil, fmt.Errorf("failed to start playback on destination: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to transfer playback (status %d): %s", resp.StatusCode, string(body))
	}

	return resp, err
}

// Helper method to pause current playback with proper error handling
func (sp *Spotify) pauseCurrentPlayback() error {
	resp, err := sp.pausePlayback()
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to pause playback (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
