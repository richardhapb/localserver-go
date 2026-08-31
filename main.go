package main

import (
	"localserver/manage"
	"localserver/server"
	"log"
)

func main() {
	if err := manage.InitializeAudio(); err != nil {
		log.Printf("Error binding audio GPIO pins: %s\n", err)
	}
	server.CreateServer()
}

