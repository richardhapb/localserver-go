package spotify

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

var currentEnv *Spotify

type Spotify struct {
	SpotifyCallbackUri string
	ClientId		   string
	ClientSecret	   string
	Devices			   []string
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
	var sp Spotify
	if environment == HOME {
		sp.ClientId = ""
		sp.ClientSecret = ""
		sp.Devices = []string{"MacBook Air de Richard", "iPhone"}
		sp.SpotifyCallbackUri = ""
	} else {
		sp.ClientId = ""
		sp.ClientSecret = ""
		sp.Devices = []string{"librespot"}
		sp.SpotifyCallbackUri = ""
	}

	return &sp
}

func updateEnv(newEnv *Spotify) {
	currentEnv = newEnv
}

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

	}

	sp := new(Environment(environment))

	updateEnv(sp)

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Loging with %s", environment),
	})
}

// def login():
//	   if not check_token():
//		   return Response("Unauthorized", 401)
//
//	   account_type = request.args.get("account")
//
//	   if not account_type or (account_type != "home" and account_type != "main"):
//		   return Response("Account is incorrect. You need to pass the account type as a URL argument: account=<account type>. It should be either home or main.", 400)
//
//	   global account
//	   account = account_type
//
//	   secrets = get_client_secrets(MAIN_DEFAULT if account == "main" else HOME_DEFAULT)
//
//	   scope = "user-read-playback-state user-modify-playback-state"
//	   auth_url = f"https://accounts.spotify.com/authorize?client_id={secrets.client_id}&response_type=code&redirect_uri={SP_CALLBACK_URI}&scope={scope}"
//	   return redirect(auth_url)
//
