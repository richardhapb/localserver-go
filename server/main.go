package server

import (
	"github.com/gin-gonic/gin"
	"localserver/spotify"
	"log"
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
	log.Println("Connecting to server...")

	router := gin.Default()
	router.SetTrustedProxies(nil)

	spotifyGroup := router.Group("/spotify")
	{
		// Public route without middleware
		spotifyGroup.GET("/login", spotify.Login)
		spotifyGroup.GET("/callback", spotify.Callback)

		// Protected routes with middleware
		protected := spotifyGroup.Group("")
		protected.Use(spotify.SpotifyMiddleware())
		{
		}
	}

	router.Run(":9000")
}

