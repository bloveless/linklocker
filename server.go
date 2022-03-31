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
	"github.com/google/uuid"
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

type tfaApplication struct {
	id                   string
	name                 string
	infobipApplicationId string
}

type tfaMessage struct {
	id               string
	applicationId    string
	name             string
	infobipMessageId string
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

		tfaApplicationName := "LinkLocker"
		auth := context.WithValue(context.Background(), infobip.ContextAPIKey, infobipApiKey)

		// First try and get the application id from the db
		var tfaApp tfaApplication
		err := db.QueryRow("SELECT id, name, infobip_application_id FROM tfa_application WHERE name = ?", tfaApplicationName).Scan(
			&tfaApp.id,
			&tfaApp.name,
			&tfaApp.infobipApplicationId,
		)

		if err != nil && err != sql.ErrNoRows {
			panic(err)
		}

		// No TFA application was found so let's create and insert one
		if err == sql.ErrNoRows {
			pinTimeToLive := "5m"
			pinAttempts := int32(5)
			allowMultiplePinVerifications := false

			tfaApplicationRequest := infobip.NewTfaApplicationRequest(tfaApplicationName)
			config := tfaApplicationRequest.GetConfiguration()
			config.PinTimeToLive = &pinTimeToLive
			config.PinAttempts = &pinAttempts
			config.AllowMultiplePinVerifications = &allowMultiplePinVerifications
			tfaApplicationRequest.SetConfiguration(config)

			tfaApplicationResponse, _, err := infobipClient.
				TfaApi.
				CreateTfaApplication(auth).
				TfaApplicationRequest(*tfaApplicationRequest).
				Execute()

			if err != nil {
				panic(err)
			}

			tfaApplicationId := uuid.New().String()

			_, err = db.Exec(
				"INSERT INTO tfa_application (id, name, infobip_application_id) VALUES (?, ?, ?)",
				tfaApplicationId,
				tfaApplicationName,
				*tfaApplicationResponse.ApplicationId,
			)

			if err != nil {
				panic(err)
			}

			infobipAppId = *tfaApplicationResponse.ApplicationId
		}

		if err == nil {
			infobipAppId = tfaApp.infobipApplicationId
		}

		tfaMessageName := "LinkLocker Default"

		// Next lets try and get the message id from the db
		var tfaMess tfaMessage
		err = db.QueryRow("SELECT id, application_id, name, infobip_message_id FROM tfa_message WHERE name = ?", tfaMessageName).Scan(
			&tfaMess.id,
			&tfaMess.applicationId,
			&tfaMess.name,
			&tfaMess.infobipMessageId,
		)

		if err != nil && err != sql.ErrNoRows {
			panic(err)
		}

		if err == sql.ErrNoRows {
			pinLength := int32(6)
			repeatCode := "5"

			createMessageRequest := infobip.NewTfaCreateMessageRequest("Your requested token for LinkLocker is {{pin}}", infobip.TFAPINTYPE_NUMERIC)
			createMessageRequest.PinLength = &pinLength
			createMessageRequest.RepeatDTMF = &repeatCode

			createMessageResponse, _, err := infobipClient.
				TfaApi.
				CreateTfaMessageTemplate(auth, infobipAppId).
				TfaCreateMessageRequest(*createMessageRequest).
				Execute()

			if err != nil {
				panic(err)
			}

			tfaMessageId := uuid.New().String()

			_, err = db.Exec(
				"INSERT INTO tfa_message (id, application_id, name, infobip_message_id) VALUES (?, ?, ?, ?)",
				tfaMessageId,
				infobipAppId,
				tfaMessageName,
				*createMessageResponse.MessageId,
			)

			if err != nil {
				panic(err)
			}

			infobipMessageId = *createMessageResponse.MessageId
		}

		if err == nil {
			infobipMessageId = tfaMess.infobipMessageId
		}
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
