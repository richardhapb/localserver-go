package main

import (
	"localserver/server"
	"localserver/manage"
	"log"
)

func main() {
	if err := manage.InitializeLamp(); err != nil {
		log.Printf("Error binding the Raspberry PI pin: %s\n", err)
	}
	server.CreateServer()
}

