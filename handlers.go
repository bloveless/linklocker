package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	openapi "github.com/twilio/twilio-go/rest/verify/v2"
)

type user struct {
	Id                uuid.UUID
	Email             string
	password          string
	Name              string
	PhoneNumber       string
	MaskedPhoneNumber string
}

type link struct {
	Id            uuid.UUID
	UserId        uuid.UUID
	SortOrder     int
	Url           string
	DisplayUrl    string
	ScreenshotUrl *string
}

type templateData struct {
	Authenticated bool
	User          *user
	Links         []link
	FormData      url.Values
	FormErrors    map[string]string
	CSRFToken     string
	PageData      map[string]interface{}
}

func (s server) home(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	return c.Render(http.StatusOK, "home.page.tmpl", td)
}

func (s server) createLink(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	linkUrl := c.FormValue("url")
	displayUrl := c.FormValue("display_url")

	if strings.TrimSpace(linkUrl) == "" {
		td.FormErrors["url"] = "URL is required"
	}

	if len(td.FormErrors) > 0 {
		return c.Render(http.StatusBadRequest, "home.page.tmpl", td)
	}

	linkUuid := uuid.New().String()
	_, err = s.db.Exec(
		"INSERT INTO link (id, user_id, sort_order, url, display_url) VALUES (?, ?, ?, ?, ?)",
		linkUuid,
		s.sessionManager.GetString(c.Request().Context(), "user_id"),
		0, // TODO: Ignoring sort order for now
		linkUrl,
		displayUrl,
	)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	s.screenshotRequests <- screenshotRequest{
		linkId:     linkUuid,
		url:        linkUrl,
		displayUrl: displayUrl,
	}

	return c.Redirect(http.StatusFound, "/")
}

func (s server) signUpForm(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	return c.Render(http.StatusOK, "sign-up.page.tmpl", td)
}

func (s server) signUp(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	email := c.FormValue("email")
	password := c.FormValue("password")
	passwordConfirmation := c.FormValue("password_confirmation")
	name := c.FormValue("name")
	phoneNumber := c.FormValue("phone_number")

	if strings.TrimSpace(email) == "" {
		td.FormErrors["email"] = "Email is required"
	}

	if _, ok := td.FormErrors["email"]; !ok {
		// If there isn't an error with the email then we can check if the email already exists
		var exists bool
		err := s.db.QueryRow("SELECT exists (SELECT email FROM user WHERE email = ?);", email).Scan(&exists)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("Internal Server Error: %w", err)
		}

		if exists {
			// TODO: Is this too much information to tell the user?
			td.FormErrors["email"] = "Email address is already in use"
		}
	}

	if strings.TrimSpace(password) == "" {
		td.FormErrors["password"] = "Password is required"
	}

	if strings.TrimSpace(passwordConfirmation) == "" {
		td.FormErrors["password_confirmation"] = "Password confirmation is required"
	}

	if password != passwordConfirmation {
		td.FormErrors["password_confirmation"] = "Password and confirmation must match"
	}

	if strings.TrimSpace(name) == "" {
		td.FormErrors["name"] = "Name is required"
	}

	if strings.TrimSpace(phoneNumber) == "" {
		td.FormErrors["phone_number"] = "Phone number is required"
	}

	if phoneNumber[0] != '+' {
		td.FormErrors["phone_number"] = "Phone number must start with a +"
	}

	if len(td.FormErrors) > 0 {
		return c.Render(http.StatusBadRequest, "sign-up.page.tmpl", td)
	}

	hashedPassword, err := generateFromPassword(password, getDefaultParams())
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	var exists bool
	err = s.db.QueryRow("SELECT exists (SELECT email FROM user WHERE email = ?);", email).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	if exists {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	userUuid := uuid.New().String()
	_, err = s.db.Exec(
		"INSERT INTO user (id, email, password, name, phone_number) VALUES (?, ?, ?, ?, ?)",
		userUuid,
		email,
		hashedPassword,
		name,
		phoneNumber,
	)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	s.sessionManager.Put(c.Request().Context(), "authenticated", false)
	s.sessionManager.Put(c.Request().Context(), "user_id", userUuid)

	return c.Redirect(http.StatusFound, "/log-in/mfa-delivery-method")
}

func (s server) logInForm(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	return c.Render(http.StatusOK, "log-in.page.tmpl", td)
}

func (s server) logIn(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	email := c.FormValue("email")
	password := c.FormValue("password")

	if strings.TrimSpace(email) == "" {
		td.FormErrors["email"] = "Email is required"
	}

	if strings.TrimSpace(password) == "" {
		td.FormErrors["password"] = "Password is required"
	}

	if len(td.FormErrors) > 0 {
		return c.Render(http.StatusBadRequest, "log-in.page.tmpl", td)
	}

	var u user
	err = s.db.QueryRow("SELECT id, email, password, name, phone_number FROM user WHERE email = ?", email).Scan(
		&u.Id,
		&u.Email,
		&u.password,
		&u.Name,
		&u.PhoneNumber,
	)

	if err != nil && errors.Is(err, sql.ErrNoRows) {
		td.FormErrors["email"] = "No account was found with this email/password combo"
		return c.Render(http.StatusBadRequest, "log-in.page.tmpl", td)
	}

	if err != nil || err == sql.ErrNoRows {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	match, err := comparePasswordAndHash(password, u.password)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	// If MFA is enabled then we don't authenticate the session yet and we redirect the user to the MFA flow
	if match && s.enableMfa {
		s.sessionManager.Put(c.Request().Context(), "authenticated", false)
		s.sessionManager.Put(c.Request().Context(), "user_id", u.Id.String())

		return c.Redirect(http.StatusFound, "/log-in/mfa-delivery-method")
	}

	// If MFA is not enabled then we immediately authenticate the session and send the user to the app
	if match && !s.enableMfa {
		s.sessionManager.Put(c.Request().Context(), "authenticated", true)
		s.sessionManager.Put(c.Request().Context(), "user_id", u.Id.String())

		return c.Redirect(http.StatusFound, "/")
	}

	return c.HTML(http.StatusNotFound, "Invalid user")
}

func (s server) logInMfaDeliveryMethodForm(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	// If there is no user_id then the user hasn't successfully provided their username and password
	if !s.sessionManager.Exists(c.Request().Context(), "user_id") {
		return c.Redirect(http.StatusFound, "/log-in")
	}

	// If the user is already authenticated then they have already completed their multi-factor login
	if s.sessionManager.GetBool(c.Request().Context(), "authenticated") {
		return c.Redirect(http.StatusFound, "/")
	}

	var maskedPhoneNumber string

	// If the user session is unauthenticated then we will only populate the users id and phone number
	if s.sessionManager.Exists(c.Request().Context(), "user_id") && !s.sessionManager.GetBool(c.Request().Context(), "authenticated") {
		userUuid := s.sessionManager.GetString(c.Request().Context(), "user_id")
		var u user
		err := s.db.QueryRow("SELECT id, phone_number FROM user WHERE id = ?", userUuid).Scan(
			&u.Id,
			&u.PhoneNumber,
		)

		if err != nil {
			return fmt.Errorf("Internal Server Error: %w", err)
		}

		maskedPhoneNumber = "XXX-XXX-" + u.PhoneNumber[len(u.PhoneNumber)-4:]
	}

	td.PageData = map[string]interface{}{
		"MaskedPhoneNumber": maskedPhoneNumber,
	}

	return c.Render(http.StatusOK, "log-in-mfa-delivery-method.page.tmpl", td)
}

func (s server) logInMfaDeliveryMethod(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	// If there is no user_id then the user hasn't successfully provided their username and password
	if !s.sessionManager.Exists(c.Request().Context(), "user_id") {
		return c.Redirect(http.StatusFound, "/")
	}

	// If the user is already authenticated then they have already completed their multi-factor login
	if s.sessionManager.GetBool(c.Request().Context(), "authenticated") {
		return c.Redirect(http.StatusFound, "/")
	}

	deliveryMethod := c.FormValue("delivery_method")

	if strings.TrimSpace(deliveryMethod) == "" {
		td.FormErrors["token_method"] = "Delivery method is required"
	}

	if deliveryMethod != "call" && deliveryMethod != "sms" {
		td.FormErrors["token_method"] = "Delivery method must be either phone or sms"
	}

	userUuid := s.sessionManager.GetString(c.Request().Context(), "user_id")
	var u user
	err = s.db.QueryRow("SELECT id, phone_number FROM user WHERE id = ?", userUuid).Scan(
		&u.Id,
		&u.PhoneNumber,
	)

	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	if len(td.FormErrors) > 0 {
		maskedPhoneNumber := "XXX-XXX-" + u.PhoneNumber[len(u.PhoneNumber)-4:]

		td.PageData = map[string]interface{}{
			"MaskedPhoneNumber": maskedPhoneNumber,
		}

		return c.Render(http.StatusOK, "log-in-mfa-delivery-method.page.tmpl", td)
	}

	// Now we have the token method as well as the users phone number so we can send the token to the user
	_, err = s.db.Exec("UPDATE tfa_token SET revoked = 1 WHERE user_id = ?", u.Id)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	expiresAt := time.Now().Add(5 * time.Minute).UTC()
	var verificationId *string

	params := &openapi.CreateVerificationParams{}
	params.SetTo(td.User.PhoneNumber)
	params.SetChannel(deliveryMethod)

	resp, err := s.twilioClient.VerifyV2.CreateVerification(os.Getenv("TWILIO_VERIFY_SERVICE_SID"), params)
	if err != nil {
		panic(fmt.Errorf("Unable to create verification: %w", err))
	}

	verificationId = resp.Sid

	if verificationId != nil {
		// We have now sent the user a token via either sms or a phone call. The token is only ever known by Infobip
		// so, we record the pin id here and that is what we will use to verify the token in a later step
		_, err = s.db.Exec(
			"INSERT INTO tfa_token (id, user_id, token_type, token, delivery_method, created_at_utc, expires_at_utc) VALUES (?, ?, ?, ?, ?, ?, ?)",
			uuid.New().String(),
			u.Id,
			"twilio_verification_id",
			verificationId,
			deliveryMethod,
			time.Now().UTC().Format("2006-01-02 15:04:05"),
			expiresAt.Format("2006-01-02 15:04:05"))

		if err != nil {
			return fmt.Errorf("Internal Server Error: %w", err)
		}
	}

	return c.Redirect(http.StatusFound, "/log-in/mfa")
}

func (s server) logInMfaForm(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	// If there is no user_id then the user hasn't successfully provided their username and password
	if !s.sessionManager.Exists(c.Request().Context(), "user_id") {
		return c.Redirect(http.StatusFound, "/log-in")
	}

	// If the user is already authenticated then they have already completed their multi-factor login
	if s.sessionManager.GetBool(c.Request().Context(), "authenticated") {
		return c.Redirect(http.StatusFound, "/")
	}

	return c.Render(http.StatusOK, "log-in-mfa.page.tmpl", td)
}

func (s server) logInMfa(c echo.Context) error {
	td, err := s.getDefaultData(c)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	// If there is no user_id then the user hasn't successfully provided their username and password
	if !s.sessionManager.Exists(c.Request().Context(), "user_id") {
		return c.Redirect(http.StatusFound, "/log-in")
	}

	// If the user is already authenticated then they have already completed their multi-factor login
	if s.sessionManager.GetBool(c.Request().Context(), "authenticated") {
		return c.Redirect(http.StatusFound, "/")
	}

	token := c.FormValue("token")

	if strings.TrimSpace(token) == "" {
		td.FormErrors["token"] = "Token is required"
	}

	if len(td.FormErrors) > 0 {
		return c.Render(http.StatusBadRequest, "log-in-mfa.page.tmpl", td)
	}

	var verificaitonId string
	err = s.db.QueryRow(
		"SELECT token FROM tfa_token WHERE user_id = ? AND revoked = 0 AND expires_at_utc >= datetime('now');",
		s.sessionManager.GetString(c.Request().Context(), "user_id"),
	).Scan(&verificaitonId)
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	params := &openapi.CreateVerificationCheckParams{}
	params.SetTo(td.User.PhoneNumber)
	params.SetCode(token)

	resp, err := s.twilioClient.VerifyV2.CreateVerificationCheck(os.Getenv("TWILIO_VERIFY_SERVICE_SID"), params)
	if err != nil {
		panic(fmt.Errorf("Unable to create verification: %w", err))
	}

	if *resp.Valid && *resp.Status != "approved" {
		td.FormErrors["token"] = fmt.Sprintf("Wrong token. Please try again")
		return c.Render(http.StatusBadRequest, "log-in-mfa.page.tmpl", td)
	}

	_, err = s.db.Exec("UPDATE tfa_token SET revoked = 1 WHERE user_id = ?", s.sessionManager.GetString(c.Request().Context(), "user_id"))
	if err != nil {
		return fmt.Errorf("Internal Server Error: %w", err)
	}

	s.sessionManager.Put(c.Request().Context(), "authenticated", true)

	return c.Redirect(http.StatusFound, "/")
}

func (s server) logOut(c echo.Context) error {
	s.sessionManager.Destroy(c.Request().Context())
	return c.Redirect(http.StatusFound, "/")
}

func (s server) getDefaultData(c echo.Context) (templateData, error) {
	td := templateData{}
	td.Authenticated = false
	td.User = nil
	td.CSRFToken = c.Get(middleware.DefaultCSRFConfig.ContextKey).(string)
	td.FormErrors = make(map[string]string)

	formData, err := c.FormParams()
	if err != nil {
		return templateData{}, fmt.Errorf("Internal Server Error: %w", err)
	}

	td.FormData = formData

	if s.sessionManager.Exists(c.Request().Context(), "user_id") {
		userUuid := s.sessionManager.GetString(c.Request().Context(), "user_id")
		var u user
		err := s.db.QueryRow("SELECT id, name, email, phone_number FROM user WHERE id = ?", userUuid).Scan(
			&u.Id,
			&u.Name,
			&u.Email,
			&u.PhoneNumber,
		)

		if err != nil {
			return templateData{}, fmt.Errorf("unable to load user_id %s: %w", userUuid, err)
		}

		linkRows, err := s.db.Query("SELECT id, user_id, sort_order, url, display_url, screenshot_url FROM link where user_id = ?", userUuid)
		if err != nil {
			return templateData{}, fmt.Errorf("unable to load links for user_id %s: %w", userUuid, err)
		}

		defer linkRows.Close()

		var links []link
		for linkRows.Next() {
			var l link
			err = linkRows.Scan(&l.Id, &l.UserId, &l.SortOrder, &l.Url, &l.DisplayUrl, &l.ScreenshotUrl)
			if err != nil {
				return templateData{}, fmt.Errorf("unable to read next link for user_id: %s: %w", userUuid, err)
			}

			links = append(links, l)
		}

		err = linkRows.Err()
		if err != nil {
			return templateData{}, fmt.Errorf("error while loading links for user_id: %s: %w", userUuid, err)
		}

		td.Authenticated = true
		td.User = &u
		td.Links = links

		return td, nil
	}

	return td, nil
}
