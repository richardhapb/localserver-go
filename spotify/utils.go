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

func (sp *Spotify) appendDeviceId(urlStr string, deviceName ...string) string {

	log.Printf("Appending device id to url: %s\n", urlStr)
	log.Printf("With device: %s\n", sp)

	var deviceId string

	if len(deviceName) > 0 {
		var id string
		var err error
		if id, err = sp.getDeviceId(deviceName[0]); err != nil {
			println(err)
		}
		deviceId = id
	}
	if deviceId == "" {
		deviceId = sp.getActiveDeviceId()
	}

	params := url.Values{}
	if deviceId != "" {
		params.Set("device_id", deviceId)
	}

	encoded := params.Encode()

	if encoded == "" {
		return urlStr
	}

	if strings.Contains(urlStr, "?") {
		return urlStr + "&" + encoded
	}

	log.Printf("Appended %s\n", encoded)
	return urlStr + "?" + encoded
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

func printResponseBody(resp *http.Response) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error decoding body: %s", err)
		return
	}

	fmt.Println(string(body))
}
