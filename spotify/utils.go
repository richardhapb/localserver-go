package spotify

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Update the current active environment
func updateEnv(newEnv *Spotify) {
	log.Println(fmt.Sprintf("Settings environment to %v", newEnv))
	currentEnv = newEnv
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
	tokens := strings.SplitSeq(dataStr, "\n")

	for token := range tokens {
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

func schedule(epochMillis int64, action func()) {
    delayMillis := epochMillis - time.Now().UnixMilli()

	log.Println(fmt.Sprintf("Scheduling task to %d seconds later", delayMillis / 1000))

	if delayMillis < 0 {
		log.Println("epochMillis is in the past in schedule function")
		return
	}

	go func() {
		time.Sleep(time.Duration(delayMillis) * time.Millisecond)
		action()
	}()
}

func printResponseBody(resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error decoding body: %s", err)
		return
	}

	fmt.Println(string(body))
}

// Extract the id from a playlist URI
// Example:
// parsePlaylistId("spotify:playlist:0qPA1tBtiCLVHCUfREECnO")
// returns "0qPA1tBtiCLVHCUfREECnO", nil
func parsePlaylistId(playlistUri string) (string, error) {
	if !strings.Contains(playlistUri, ":playlist:") {
		return "", fmt.Errorf("Playlist URI is invalid: %s", playlistUri)
	}

	parts := strings.Split(playlistUri, ":")

	if len(parts) != 3 {
		return "", fmt.Errorf("Playlist URI is invalid: %s", playlistUri)
	}

	return parts[2], nil
}
