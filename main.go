package main

import (
	"log"
	"net/http"
)

func main() {
	server := newServer()

	log.Println("Starting server on 0.0.0.0:3000")
	if err := http.ListenAndServe("0.0.0.0:3000", server.getRouter()); err != nil {
		panic(err)
	}
}
