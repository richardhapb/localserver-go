package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"localserver/spotify"
)

// @app.route("/richard/login", methods=["POST"])
// @app.route("/spotify/pause")
// @app.route("/schedule")
// @app.route("/spotify/login")
// @app.route("/spotify/callback")
// @app.route("/spotify/playlist")
// @app.route("/spotify/volume")
// @app.route("/spotify/get-device", methods=["POST"])


func CreateServer() {
	router := gin.Default()

	SPOTIFY := "spotify"

	router.GET(fmt.Sprintf("%s/login", SPOTIFY), spotify.Login)

	router.Run()
}

