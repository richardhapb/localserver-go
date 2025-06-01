package server

import (
	"github.com/gin-gonic/gin"
	"localserver/spotify"
	"localserver/manage"
	"log"
)

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
			protected.GET("/play", spotify.Play)
			protected.GET("/pause", spotify.Pause)
			protected.GET("/schedule", spotify.Schedule)
			protected.GET("/playlist", spotify.PlayPlaylist)
			protected.GET("/volume", spotify.Volume)
			protected.GET("/transfer", spotify.TransferPlayback)
		}
	}

	manageGroup := router.Group("/manage")
	{
		manageGroup.GET("/wake", manage.Wake)
		manageGroup.GET("/sleep", manage.Sleep)
		manageGroup.GET("/battery", manage.Battery)
		manageGroup.GET("/lamp", manage.ToggleLamp)
		manageGroup.POST("/jn", manage.LaunchJn)
	}

	router.Run(":9000")
}
