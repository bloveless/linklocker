package main

import (
	"context"
	"database/sql"
	"html/template"
	"io"
	"os"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/labstack/echo/v4"
	_ "github.com/mattn/go-sqlite3"
	"github.com/twilio/twilio-go"
)

type screenshotRequest struct {
	linkId     string
	url        string
	displayUrl string
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	if data == nil {
		data = templateData{}
	}

	return t.templates.ExecuteTemplate(w, name, data)
}

type server struct {
	db                 *sql.DB
	sessionManager     *scs.SessionManager
	screenshotRequests chan screenshotRequest
	chromeDpContext    context.Context
	enableMfa          bool
	twilioClient       *twilio.RestClient
	templates          *Template
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

	err = m.Up()
	if err != nil && err != migrate.ErrNoChange {
		panic(err)
	}

	screenshotRequests := make(chan screenshotRequest)

	enableMfa := os.Getenv("ENABLE_MFA") == "true"

	sessionManager := scs.New()
	sessionManager.Lifetime = 24 * time.Hour
	sessionManager.Cookie.HttpOnly = true

	ts, err := template.New("templates").
		Funcs(template.FuncMap{"mod": func(i, j int) bool { return (i+1)%j == 0 }}).
		ParseGlob("views/*.tmpl")
	if err != nil {
		panic(err)
	}

	var twilioClient *twilio.RestClient
	if enableMfa {
		twilioClient = twilio.NewRestClientWithParams(twilio.ClientParams{
			Username: os.Getenv("TWILIO_SID"),
			Password: os.Getenv("TWILIO_AUTH_TOKEN"),
		})
	}

	return server{
		db:                 db,
		sessionManager:     sessionManager,
		screenshotRequests: screenshotRequests,
		chromeDpContext:    chromeCtx,
		twilioClient:       twilioClient,
		enableMfa:          enableMfa,
		templates:          &Template{templates: ts},
	}
}
