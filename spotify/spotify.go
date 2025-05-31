package spotify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
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
	ID             string `json:"id"`
	Name           string `json:"name"`
	IsActive       bool   `json:"is_active"`
	VolumenPercent int    `json:"volume_percent"`
	SupportsVolume bool   `json:"supports_volume"`
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
	ProgressMs int   `json:"progress_ms"`
	IsPlaying  bool  `json:"is_playing"`
	Timestamp  int   `json:"timestamp"`
	Item       Track `json:"item"`
}

type Playlist struct {
	Tracks struct {
		Total int     `json:"total"`
		Limit int     `json:"limit"`
		Items []Track `json:"items"`
	} `json:"tracks"`
	Description string `json:"description"`
	Name        string `json:"name"`
	Public      bool   `json:"public"`
}

type Track struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Uri        string `json:"uri"`
	DurationMs int    `json:"duration_ms"`
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
		log.Println("Returning existent Spotify instance")
		return envs[string(environment)]
	}
	log.Println("Creating new Spotify instance")

	if err := godotenv.Load(); err != nil {
		log.Fatalln(".env not found")
		return nil
	}

	var sp Spotify
	envPrefix := ""
	defaultVolume := 50

	switch environment {
	case Main:
		envPrefix = "MAIN_"
		sp.Devices = []Device{{"", "iPhone", false, defaultVolume, false}, {"", "MacBook Air de Richard", false, defaultVolume, true}}
	case Home:
		envPrefix = "HOME_"
		sp.Devices = []Device{{"", "librespot", false, defaultVolume, true}}
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
	devices, err := getDevicesData(sp.tokens.AccessToken)

	if err != nil {
		return nil
	}

	sp.Devices = devices

	for _, device := range devices {
		if device.IsActive {
			return &device
		}
	}

	// Return the first one as the default
	return &sp.Devices[0]
}

func (sp *Spotify) getActiveDeviceId() string {
	if device := sp.getActiveDevice(); device != nil {
		if device.ID != "" {
			return device.ID
		}
	}

	return ""
}

func (sp *Spotify) getDeviceId(deviceName string) (string, error) {

	if deviceName == "" {
		return "", fmt.Errorf("device name is empty")
	}

	log.Printf("Retrieving id for device %s\n", deviceName)

	devices, err := getDevicesData(sp.tokens.AccessToken)

	if err != nil {
		return "", err
	}

	deviceId := ""
	log.Printf("Devices found: %v", devices)

	for _, device := range devices {
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

	if resp.StatusCode == http.StatusBadRequest {
		fmt.Println("Bad request:")
		printResponseBody(resp)
	}

	return resp, nil
}

func (sp *Spotify) appendDeviceId(baseUrl string) string {
	deviceId := sp.getActiveDeviceId()
	if deviceId == "" {
		return baseUrl
	}
	u, err := url.Parse(baseUrl)
	if err != nil {
		log.Printf("Error parsing URL: %v", err)
		return baseUrl
	}
	q := u.Query()
	q.Set("device_id", deviceId)
	u.RawQuery = q.Encode()
	return u.String()
}

func (sp *Spotify) setVolume(volumePercent int) (*http.Response, error) {
	device := sp.getActiveDevice()

	if device == nil {
		return nil, fmt.Errorf("device not found")
	}

	log.Println(device)

	if !device.SupportsVolume {
		return nil, fmt.Errorf("device doesn't support volume")
	}

	log.Printf("Setting volume to %d", volumePercent)

	baseUrl := "https://api.spotify.com/v1/me/player/volume"
	deviceId, err := sp.getDeviceId(device.Name)
	if err != nil {
		return nil, fmt.Errorf("Error getting device Id: %s", err)
	}

	params := url.Values{}
	params.Set("volume_percent", strconv.Itoa(volumePercent))
	params.Set("device_id", deviceId)

	urlStr := baseUrl + "?" + params.Encode()

	return sp.makeRequest("PUT", urlStr)
}

func (sp *Spotify) playPlaylist(contextUri string, volumePercent int, args ...int) (*http.Response, error) {
	log.Printf("Playing list with URI %s", contextUri)

	requestBody := map[string]any{
		"context_uri": contextUri,
		"position_ms": 0,
	}
	if len(args) > 0 {
		requestBody["offset"] = map[string]int{
			"position": args[0],
		}
	} else {
		id, err := parsePlaylistId(contextUri)

		if err == nil {
			// Get the length of the playlist to select a random track
			baseUrl := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s", id)
			query := url.Values{
				"fields": {"tracks"},
				"limit":  {"1"},
				"offset": {"0"},
			}
			urlStr := baseUrl + "?" + query.Encode()

			resp, err := sp.makeRequest("GET", urlStr)

			if err != nil {
				return nil, fmt.Errorf("Failed to marshal the response body while retrieving the playlist: %w", err)
			}

			var playlist Playlist

			err = json.NewDecoder(resp.Body).Decode(&playlist)
			resp.Body.Close()
			if err != nil {
				log.Printf("Failed to decode response: %s", err)
				return nil, err
			}

			requestBody["offset"] = map[string]int{
				"position": rand.Intn(playlist.Tracks.Total),
			}
		}
	}

	if len(args) > 1 {
		requestBody["position_ms"] = args[1]
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	urlStr := sp.appendDeviceId(PlayEndpoint)

	device := sp.getActiveDevice()

	if device != nil && device.SupportsVolume {
		sp.setVolume(volumePercent)
	}

	go func() {
		time.Sleep(5 * time.Second)
		sp.toggleShuffle(true)
		sp.enableRepeat("context")
	}()

	return sp.makeRequest("PUT", urlStr, jsonBody)
}

func (sp *Spotify) playPlayback() (*http.Response, error) {
	urlStr := sp.appendDeviceId(PlayEndpoint)

	return sp.makeRequest("PUT", urlStr)
}

func (sp *Spotify) pausePlayback() (*http.Response, error) {
	baseUrl := "https://api.spotify.com/v1/me/player/pause"

	urlStr := sp.appendDeviceId(baseUrl)

	return sp.makeRequest("PUT", urlStr)
}

func getEnvFromDeviceName(deviceName string) *Spotify {
	if deviceName == "" {
		return nil
	}

	log.Printf("Retrieving data for device name: %s\n", deviceName)

	// Loop through environments checking device lists
	for _, env := range envs {
		if len(env.Devices) == 0 {
			devices, err := getDevicesData(env.tokens.AccessToken)
			if err != nil {
				log.Printf("Error retrieving devices data: %s\n", err)
			} else {
				env.Devices = devices
			}
		}
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

func (sp *Spotify) toggleShuffle(state bool) {
	stateStr := ""
	if state {
		stateStr = "true"
	} else {
		stateStr = "false"
	}

	baseUrl := fmt.Sprintf("https://api.spotify.com/v1/me/player/shuffle?state=%s", stateStr)
	urlStr := sp.appendDeviceId(baseUrl)

	sp.makeRequest("PUT", urlStr)
}

// Possibles states:
// track, context or off.
// track will repeat the current track.
// context will repeat the current context.
// off will turn repeat off.
// Example: state=context
func (sp *Spotify) enableRepeat(state string) {
	baseUrl := fmt.Sprintf("https://api.spotify.com/v1/me/player/repeat?state=%s", state)

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

func (sp *Spotify) hardTransferPlayback(to *Spotify) error {
	if to == nil {
		return fmt.Errorf("destination Spotify instance is nil")
	}

	playback, err := sp.getCurrentPlayback()

	if err != nil {
		return fmt.Errorf("error retrieving currrent playback: %s", err)
	}

	volume := 50 // TODO: Make this dynamic

	if playback.Device.SupportsVolume {
		// Get the volume if it is supported
		volume = playback.Device.VolumenPercent
	}

	// Transfer current track
	err = sp.pauseCurrentPlayback()
	if err != nil {
		return fmt.Errorf("error pausing current playback")
	}

	if playback.Context.Uri == "" {
		return fmt.Errorf("There is no context currently playing.")
	}

	trackNumber := to.getTrackNumber(playback.Context.Uri, playback.Item.Name)
	resp, err := to.playPlaylist(playback.Context.Uri, volume, trackNumber, playback.ProgressMs)

	if err != nil {
		return fmt.Errorf("error playing uris: %s", err)
	}

	defer resp.Body.Close()

	return nil
}

func (sp *Spotify) getTrackNumber(playlistUri, trackName string) int {
	if playlistUri == "" || trackName == "" {
		return 0
	}

	playlistId, err := parsePlaylistId(playlistUri)
	if err != nil {
		log.Printf("Error parsing playlist id: %s", err)
		return 0
	}

	// Spotify API endpoint
	baseUrl := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks", playlistId)

	// Query parameters
	query := url.Values{
		"fields": {"items(track(name)),next"}, // Only fetch track names
		"limit":  {"100"},                     // Maximum allowed by Spotify
		"offset": {"0"},
	}

	type trackPage struct {
		Items []struct {
			Track struct {
				Name string `json:"name"`
			} `json:"track"`
		} `json:"items"`
		Next string `json:"next"`
	}

	offset := 0
	for {
		query.Set("offset", strconv.Itoa(offset))
		urlStr := baseUrl + "?" + query.Encode()

		resp, err := sp.makeRequest("GET", urlStr)
		if err != nil {
			log.Printf("Failed to fetch tracks: %s", err)
			return 0
		}

		var page trackPage
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			log.Printf("Failed to decode response: %s", err)
			return 0
		}

		// Search for track in current page
		for i, item := range page.Items {
			if item.Track.Name == trackName {
				return offset + i
			}
		}

		// Break if no more pages
		if page.Next == "" {
			break
		}

		offset += len(page.Items)
	}

	return 0
}
