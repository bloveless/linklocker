package main

import (
	"context"
	"database/sql"
	"fmt"
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
	enableMfa          bool
	infobipClient      *infobip.APIClient
	infobipHost        string
	infobipApiKey      string
	infobipAppId       string
	infobipMessageId   string
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

	enableMfa := os.Getenv("ENABLE_MFA") == "true"

	infobipHost := ""
	infobipApiKey := ""
	infobipAppId := ""
	infobipMessageId := ""

	var infobipClient *infobip.APIClient

	if enableMfa {
		infobipHost = os.Getenv("INFOBIP_HOST")

		configuration := infobip.NewConfiguration()
		configuration.Host = infobipHost

		infobipClient = infobip.NewAPIClient(configuration)
		infobipApiKey = os.Getenv("INFOBIP_API_KEY")

		auth := context.WithValue(context.Background(), infobip.ContextAPIKey, infobipApiKey)

		newApplicationRequest := infobip.NewTfaApplicationRequest("LinkLocker")
		newApplicationResponse, newApplicationHttpResponse, err := infobipClient.
			TfaApi.
			CreateTfaApplication(auth).
			TfaApplicationRequest(*newApplicationRequest).
			Execute()

		if err != nil {
			panic(err)
		}

		fmt.Printf("New Application Response %+v", newApplicationResponse)
		fmt.Printf("New Application Http Response %+v", newApplicationHttpResponse)

		pinLength := int32(6)
		repeatCode := "5"

		createMessageRequest := infobip.NewTfaCreateMessageRequest("Your requested token for LinkLocker is {{pin}}", infobip.TFAPINTYPE_ALPHANUMERIC)
		createMessageRequest.PinLength = &pinLength
		createMessageRequest.RepeatDTMF = &repeatCode

		createMessageResponse, createMessageHttpResponse, err := infobipClient.
			TfaApi.
			CreateTfaMessageTemplate(auth, *newApplicationResponse.ApplicationId).
			TfaCreateMessageRequest(*createMessageRequest).
			Execute()

		if err != nil {
			panic(err)
		}

		fmt.Printf("Create Message Response %+v\n", createMessageResponse)
		fmt.Printf("Create Message Http Response %+v\n", createMessageHttpResponse)

		infobipAppId = *newApplicationResponse.ApplicationId
		infobipMessageId = *createMessageResponse.MessageId
	}

	return server{
		db:                 db,
		session:            session,
		screenshotRequests: screenshotRequests,
		chromeDpContext:    chromeCtx,
		infobipClient:      infobipClient,
		enableMfa:          enableMfa,
		infobipHost:        "https://" + infobipHost,
		infobipApiKey:      infobipApiKey,
		infobipAppId:       infobipAppId,
		infobipMessageId:   infobipMessageId,
	}
}
