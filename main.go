package main

import (
	"context"
	"log"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Unable to load .env file. Proceeding without it")
	}

	chromeURL := os.Getenv("CHROME_URL")

	var chromeCtx context.Context
	if chromeURL != "" {
		log.Println("Starting to get screenshot")
		// create context
		allocatorContext, allocatorCancel := chromedp.NewRemoteAllocator(context.Background(), chromeURL)
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

	e := server.getRouter()
	e.Renderer = server.templates
	e.Debug = true

	log.Println("Starting server on 0.0.0.0:3000")
	e.Logger.Error(e.Start("0.0.0.0:3000"))
}
