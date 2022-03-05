package main

import (
	"context"
	"io/ioutil"
	"log"

	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
)

func (s server) screenshotWorker(chromeDpContext context.Context, requests chan screenshotRequest) {
	log.Println("Starting screenshot worker")
	for request := range requests {
		log.Printf("Processing request in worker %v", request)

		var buf []byte
		// capture entire browser viewport, returning png with quality=90
		requestUrl := request.url
		if request.displayUrl != "" {
			requestUrl = request.displayUrl
		}

		if err := chromedp.Run(chromeDpContext, fullScreenshot(requestUrl, 90, &buf)); err != nil {
			log.Fatal(err)
		}

		screenshotUrl := "static/screenshots/" + uuid.New().String() + ".png"
		if err := ioutil.WriteFile(screenshotUrl, buf, 0o644); err != nil {
			log.Fatal(err)
		}

		_, err := s.db.Exec("UPDATE link SET screenshot_url = ? WHERE id = ?", "/"+screenshotUrl, request.linkId)
		if err != nil {
			panic(err)
		}

		log.Printf("wrote " + screenshotUrl + " for link id " + request.linkId)
	}

	log.Println("Screenshot worker terminating")
}

// fullScreenshot takes a screenshot of the entire browser viewport.
//
// Note: chromedp.FullScreenshot overrides the device's emulation settings. Use
// device.Reset to reset the emulation and viewport settings.
func fullScreenshot(urlstr string, quality int, res *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.Navigate(urlstr),
		chromedp.FullScreenshot(res, quality),
	}
}

func (s server) startScreenshotRequestProcessor() {
	go s.screenshotWorker(s.chromeDpContext, s.screenshotRequests)

	rows, err := s.db.Query("SELECT id, url, display_url FROM link where link.screenshot_url IS NULL")
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var request screenshotRequest

		if err := rows.Scan(&request.linkId, &request.url, &request.displayUrl); err != nil {
			panic(err)
		}

		log.Printf("Processing backlog request: %v", request)
		s.screenshotRequests <- request
	}
}
