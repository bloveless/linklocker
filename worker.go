package main

import (
	"context"
	"io/ioutil"
	"log"

	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
)

func (s server) getScreenshotUrl(chromeDpContext context.Context, requestUrl string) (string, error) {
	var buf []byte
	// capture entire browser viewport, returning jpeg with quality=90
	screenshotPath := "static/screenshots/" + uuid.New().String() + ".jpeg"
	if err := chromedp.Run(chromeDpContext, fullScreenshot(requestUrl, 90, &buf)); err != nil {
		log.Printf("error generating screenshot for %v: %v", requestUrl, err)
		return "", err
	}

	if err := ioutil.WriteFile(screenshotPath, buf, 0o644); err != nil {
		log.Printf("error writing screenshot to file %v for %v: %v", screenshotPath, requestUrl, err)
		return "", err
	}

	return screenshotPath, nil
}

func (s server) screenshotWorker(chromeDpContext context.Context, requests chan screenshotRequest) {
	log.Println("Starting screenshot worker")
	for request := range requests {
		log.Printf("Processing request in worker %v", request)

		requestUrl := request.url
		if request.displayUrl != "" {
			requestUrl = request.displayUrl
		}

		// Get screenshot path
		screenshotPath, err := s.getScreenshotUrl(chromeDpContext, requestUrl)
		if err != nil {
			screenshotPath = "static/img/error.png"
		}

		_, err = s.db.Exec("UPDATE link SET screenshot_url = ? WHERE id = ?", "/"+screenshotPath, request.linkId)
		if err != nil {
			log.Printf("error when updating screenshot url to %s for link %s: %v", "/"+screenshotPath, request.linkId, err)
			continue
		}

		log.Printf("wrote " + screenshotPath + " for link id " + request.linkId)
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

	var requests []screenshotRequest
	for rows.Next() {
		var request screenshotRequest

		if err := rows.Scan(&request.linkId, &request.url, &request.displayUrl); err != nil {
			panic(err)
		}

		log.Printf("Processing backlog request: %v", request)
		requests = append(requests, request)
	}

	err = rows.Close()
	if err != nil {
		panic(err)
	}

	for _, request := range requests {
		s.screenshotRequests <- request
	}
}
