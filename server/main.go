package server

import (
	"context"
	"github.com/gin-gonic/gin"
	"localserver/corrections"
	"localserver/manage"
	"localserver/spotify"
	"log"
)

func CreateServer() {
	log.Println("Connecting to server...")

	if err := corrections.Init(context.Background()); err != nil {
		log.Printf("corrections: init failed, endpoint will return 503: %v", err)
	}
	defer corrections.Close()

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
			protected.GET("/search-playlist", spotify.SearchAndPlayPlaylist)
			protected.GET("/volume", spotify.Volume)
			protected.GET("/transfer", spotify.TransferPlayback)
		}
	}

	manageGroup := router.Group("/manage")
	{
		manageGroup.GET("/lamp", manage.ToggleLamp)
		manageGroup.POST("/grammar", manage.ReviewGrammar)
	}

	router.GET("/corrections", corrections.List)

	router.Run(":9000")
}
