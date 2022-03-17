package main

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golangcollege/sessions"
	"github.com/infobip/infobip-api-go-client/v2"
	_ "github.com/mattn/go-sqlite3"
)

type screenshotRequest struct {
	linkId     string
	url        string
	displayUrl string
}

type server struct {
	db                 *sql.DB
	session            *sessions.Session
	screenshotRequests chan screenshotRequest
	chromeDpContext    context.Context
	infobipClient      *infobip.APIClient
	infobipApiKey      string
}

func newServer(chromeCtx context.Context) server {
	db, err := sql.Open("sqlite3", "linklocker.sqlite")
	if err != nil {
		panic(err)
	}

	_, err = db.Query(`
		PRAGMA strict = ON;
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 5000;
		PRAGMA foreign_keys = ON;
	`)
	if err != nil {
		panic(err)
	}

	driver, err := sqlite3.WithInstance(db, &sqlite3.Config{})
	if err != nil {
		panic(err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://migrations", "sqlite3", driver)
	if err != nil {
		panic(err)
	}

	session := sessions.New([]byte(os.Getenv("SESSION_SECRET")))
	session.Lifetime = 24 * time.Hour
	session.HttpOnly = true

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		panic(err)
	}

	screenshotRequests := make(chan screenshotRequest)

	configuration := infobip.NewConfiguration()
	configuration.Host = "pw44yv.api.infobip.com"

	infobipClient := infobip.NewAPIClient(configuration)

	return server{
		db:                 db,
		session:            session,
		screenshotRequests: screenshotRequests,
		chromeDpContext:    chromeCtx,
		infobipClient:      infobipClient,
		infobipApiKey:      "9636fd1b5e74bad50c4814a9359fbdae-367e093b-436a-4d96-90b8-caa4bb27a523",
	}
}
