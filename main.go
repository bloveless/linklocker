package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Unable to load .env file. Proceeding without it")
	}

	chromeUrl := os.Getenv("CHROME_URL")

	var chromeCtx context.Context
	if chromeUrl != "" {
		log.Println("Starting to get screenshot")
		// create context
		allocatorContext, allocatorCancel := chromedp.NewRemoteAllocator(context.Background(), "ws://127.0.0.1:9222/")
		defer allocatorCancel()

		currentContext, cancel := chromedp.NewContext(allocatorContext)
		defer cancel()

		chromeCtx = currentContext
	} else {
		currentContext, cancel := chromedp.NewContext(context.Background())
		defer cancel()

		chromeCtx = currentContext
	}

	server := newServer(chromeCtx)
	server.startScreenshotRequestProcessor()

	log.Println("Starting server on 0.0.0.0:3000")
	if err := http.ListenAndServe("0.0.0.0:3000", server.getRouter()); err != nil {
		panic(err)
	}
}
