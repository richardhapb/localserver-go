package spotify

import (
	"strings"
	"os"
	"fmt"
	"log"
	"errors"
)

// Update the current active environment
func updateEnv(newEnv *Spotify) {
	log.Println(fmt.Sprintf("Settings environment to %s", newEnv.Devices))
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

