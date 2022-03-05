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

	return server{
		db:                 db,
		session:            session,
		screenshotRequests: screenshotRequests,
		chromeDpContext:    chromeCtx,
	}
}
