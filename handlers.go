package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/csrf"
	"github.com/infobip/infobip-api-go-client/v2"
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
	CSRFTag       template.HTML
	PageData      map[string]interface{}
}

func (s server) home(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "home.page.tmpl", nil)
}

func (s server) createLink(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	linkUrl := r.FormValue("url")
	displayUrl := r.FormValue("display_url")

	formErrors := make(map[string]string)

	if strings.TrimSpace(linkUrl) == "" {
		formErrors["url"] = "URL is required"
	}

	if len(formErrors) > 0 {
		s.render(w, r, "home.page.tmpl", &templateData{
			FormData:   r.PostForm,
			FormErrors: formErrors,
		})
	}

	linkUuid := uuid.New().String()
	_, err = s.db.Exec(
		"INSERT INTO link (id, user_id, sort_order, url, display_url) VALUES (?, ?, ?, ?, ?)",
		linkUuid,
		s.session.GetString(r, "user_id"),
		0, // TODO: Ignoring display order for now
		linkUrl,
		displayUrl,
	)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.screenshotRequests <- screenshotRequest{
		linkId:     linkUuid,
		url:        linkUrl,
		displayUrl: displayUrl,
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func (s server) signUpForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "sign-up.page.tmpl", nil)
}

func (s server) signUp(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	passwordConfirmation := r.FormValue("password_confirmation")
	name := r.FormValue("name")
	phoneNumber := r.FormValue("phone_number")

	formErrors := make(map[string]string)

	if strings.TrimSpace(email) == "" {
		formErrors["email"] = "Email is required"
	}

	if _, ok := formErrors["email"]; !ok {
		// If there isn't an error with the email then we can check if the email already exists
		var exists bool
		err = s.db.QueryRow("SELECT exists (SELECT email FROM user WHERE email = ?);", email).Scan(&exists)
		if err != nil && err != sql.ErrNoRows {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if exists {
			// TODO: Is this too much information to tell the user?
			formErrors["email"] = "Email address is already in use"
		}
	}

	if strings.TrimSpace(password) == "" {
		formErrors["password"] = "Password is required"
	}

	if strings.TrimSpace(passwordConfirmation) == "" {
		formErrors["password_confirmation"] = "Password confirmation is required"
	}

	if password != passwordConfirmation {
		formErrors["password_confirmation"] = "Password and confirmation must match"
	}

	if strings.TrimSpace(name) == "" {
		formErrors["name"] = "Name is required"
	}

	if strings.TrimSpace(phoneNumber) == "" {
		formErrors["phone_number"] = "Phone number is required"
	}

	if len(formErrors) > 0 {
		s.render(w, r, "sign-up.page.tmpl", &templateData{
			FormData:   r.PostForm,
			FormErrors: formErrors,
		})
		return
	}

	hashedPassword, err := generateFromPassword(password, getDefaultParams())
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var exists bool
	err = s.db.QueryRow("SELECT exists (SELECT email FROM user WHERE email = ?);", email).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if exists {
		log.Println(err)
		http.Error(w, "Email address is already in use", http.StatusConflict)
		return
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
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.session.Put(r, "authenticated", false)
	s.session.Put(r, "user_id", userUuid)

	http.Redirect(w, r, "/log-in/sms", http.StatusFound)
}

func (s server) logInForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "log-in.page.tmpl", nil)
}

func (s server) logIn(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	email := r.Form.Get("email")
	password := r.Form.Get("password")

	formErrors := make(map[string]string)

	if strings.TrimSpace(email) == "" {
		formErrors["email"] = "Email is required"
	}

	if strings.TrimSpace(password) == "" {
		formErrors["password"] = "Password is required"
	}

	if len(formErrors) > 0 {
		s.render(w, r, "log-in.page.tmpl", &templateData{
			FormData:   r.PostForm,
			FormErrors: formErrors,
		})
		return
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
		formErrors["email"] = "No account was found with this email/password combo"
		s.render(w, r, "log-in.page.tmpl", &templateData{
			FormData:   r.PostForm,
			FormErrors: formErrors,
		})
		return
	}

	if err != nil || err == sql.ErrNoRows {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	match, err := comparePasswordAndHash(password, u.password)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if match {
		s.session.Put(r, "authenticated", false)
		s.session.Put(r, "user_id", u.Id.String())

		http.Redirect(w, r, "/log-in/choose-mfa", http.StatusFound)
	} else {
		if _, err = w.Write([]byte("Invalid user")); err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

func (s server) logInChooseMfaForm(w http.ResponseWriter, r *http.Request) {
	// If there is no user_id then the user hasn't successfully provided their username and password
	if !s.session.Exists(r, "user_id") {
		http.Redirect(w, r, "/log-in", http.StatusFound)
		return
	}

	// If the user is already authenticated then they have already completed their multi-factor login
	if s.session.GetBool(r, "authenticated") {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	var maskedPhoneNumber string

	// If the user session is unauthenticated then we will only populate the users id and phone number
	if s.session.Exists(r, "user_id") && !s.session.GetBool(r, "authenticated") {
		userUuid := s.session.GetString(r, "user_id")
		var u user
		err := s.db.QueryRow("SELECT id, phone_number FROM user WHERE id = ?", userUuid).Scan(
			&u.Id,
			&u.PhoneNumber,
		)

		if err != nil {
			log.Println(err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		maskedPhoneNumber = "XXX-XXX-" + u.PhoneNumber[len(u.PhoneNumber)-4:]
	}

	td := templateData{
		PageData: map[string]interface{}{
			"MaskedPhoneNumber": maskedPhoneNumber,
		},
	}

	s.render(w, r, "log-in-choose-mfa.page.tmpl", &td)
}

func (s server) logInChooseMfa(w http.ResponseWriter, r *http.Request) {
	// If there is no user_id then the user hasn't successfully provided their username and password
	if !s.session.Exists(r, "user_id") {
		http.Redirect(w, r, "/log-in", http.StatusFound)
		return
	}

	// If the user is already authenticated then they have already completed their multi-factor login
	if s.session.GetBool(r, "authenticated") {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	err := r.ParseForm()
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	tokenMethod := r.Form.Get("token_method")

	formErrors := make(map[string]string)

	if strings.TrimSpace(tokenMethod) == "" {
		formErrors["token_method"] = "Token method is required"
	}

	if tokenMethod != "phone" && tokenMethod != "sms" {
		formErrors["token_method"] = "Token method must be either phone or sms"
	}

	userUuid := s.session.GetString(r, "user_id")
	var u user
	err = s.db.QueryRow("SELECT id, phone_number FROM user WHERE id = ?", userUuid).Scan(
		&u.Id,
		&u.PhoneNumber,
	)

	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(formErrors) > 0 {
		maskedPhoneNumber := "XXX-XXX-" + u.PhoneNumber[len(u.PhoneNumber)-4:]

		td := templateData{
			PageData: map[string]interface{}{
				"MaskedPhoneNumber": maskedPhoneNumber,
			},
		}

		td.FormData = r.PostForm
		td.FormErrors = formErrors

		s.render(w, r, "log-in-choose-mfa.page.tmpl", &td)
		return
	}

	// Now we have the token method as well as the users phone number so we can send the token to the user
	_, err = s.db.Exec("UPDATE two_factor_token SET revoked = 1 WHERE user_id = ?", u.Id)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	token := generateMfaToken(6)
	expiresAt := time.Now().Add(5 * time.Minute).UTC()

	// Now the user has provided their correct username and password so we can send their MFA code via sms
	// and wait for them to provide it to authenticate their login
	_, err = s.db.Exec(
		"INSERT INTO two_factor_token (id, user_id, token, expires_at_utc) VALUES (?, ?, ?, ?)",
		uuid.New().String(),
		u.Id,
		token,
		expiresAt.Format("2006-01-02 15:04:05"))
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if tokenMethod == "sms" {
		auth := context.WithValue(r.Context(), infobip.ContextAPIKey, s.infobipApiKey)
		request := infobip.NewSmsAdvancedTextualRequest()
		destination := infobip.NewSmsDestination(u.PhoneNumber)

		from := "LinkLocker"
		text := "Your requested token for LinkLocker is: " + token
		message := infobip.NewSmsTextualMessage()
		message.From = &from
		message.Destinations = &[]infobip.SmsDestination{*destination}
		message.Text = &text

		request.Messages = &[]infobip.SmsTextualMessage{*message}

		_, httpResponse, err := s.infobipClient.
			SendSmsApi.
			SendSmsMessage(auth).
			SmsAdvancedTextualRequest(*request).
			Execute()

		if err != nil {
			apiErr, isApiErr := err.(infobip.GenericOpenAPIError)
			if isApiErr {
				ibErr, isIbErr := apiErr.Model().(infobip.SmsApiException)
				if isIbErr {
					fmt.Println(ibErr.RequestError.ServiceException.GetMessageId())
					fmt.Println(ibErr.RequestError.ServiceException.GetText())
				}
			}
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		log.Printf("httpResponse.StatusCode: %v\n", httpResponse.StatusCode)
		log.Printf("httpResponse.Body: %v\n", httpResponse.Body)
	}

	if tokenMethod == "phone" {
		// The token needs to have spaces between each number so the numbers are read one by one rather than as one
		// whole number
		message := "Your requested token for LinkLocker is " + strings.Join(strings.Split(token, ""), " ") + ". Please press 5 to repeat."
		payload := strings.NewReader(fmt.Sprintf(`{
			"bulkId": "%s",
			"messages": [{
				"text":"%s",
				"language":"en",
				"voice":{
					"name":"Joanna",
					"gender":"female"
				},
				"repeatDtmf": "5",
				"maxDtmf": 1,
				"dtmfTimeout": 5,
				"from":"442032864231",
				"destinations": [{
					"to":"%s"
				}]
			}]
		}`, uuid.New().String(), message, u.PhoneNumber))

		client := &http.Client{
			Timeout: 30 * time.Second,
		}
		req, err := http.NewRequest("POST", s.infobipHost+"/tts/3/advanced", payload)

		if err != nil {
			fmt.Println(err)
			return
		}
		req.Header.Add("Authorization", "App "+s.infobipApiKey)
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Accept", "application/json")

		res, err := client.Do(req)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer res.Body.Close()

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println(string(body))
	}

	http.Redirect(w, r, "/log-in/mfa", http.StatusFound)
}

func (s server) logInMfaForm(w http.ResponseWriter, r *http.Request) {
	// If there is no user_id then the user hasn't successfully provided their username and password
	if !s.session.Exists(r, "user_id") {
		http.Redirect(w, r, "/log-in", http.StatusFound)
		return
	}

	// If the user is already authenticated then they have already completed their multi-factor login
	if s.session.GetBool(r, "authenticated") {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	s.render(w, r, "log-in-mfa.page.tmpl", nil)
}

func (s server) logInMfa(w http.ResponseWriter, r *http.Request) {
	// If there is no user_id then the user hasn't successfully provided their username and password
	if !s.session.Exists(r, "user_id") {
		http.Redirect(w, r, "/log-in", http.StatusFound)
		return
	}

	// If the user is already authenticated then they have already completed their multi-factor login
	if s.session.GetBool(r, "authenticated") {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	err := r.ParseForm()
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	token := r.Form.Get("token")

	formErrors := make(map[string]string)

	if strings.TrimSpace(token) == "" {
		formErrors["token"] = "Token is required"
	}

	if len(formErrors) > 0 {
		s.render(w, r, "log-in-mfa.page.tmpl", &templateData{
			FormData:   r.PostForm,
			FormErrors: formErrors,
		})
		return
	}

	var exists bool
	err = s.db.QueryRow("SELECT exists (SELECT user_id FROM two_factor_token WHERE user_id = ? AND token = ? AND revoked = 0 AND expires_at_utc >= datetime('now'));", s.session.GetString(r, "user_id"), token).Scan(&exists)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if !exists {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	_, err = s.db.Exec("UPDATE two_factor_token SET revoked = 1 WHERE user_id = ?", s.session.GetString(r, "user_id"))
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	s.session.Put(r, "authenticated", true)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s server) logOut(w http.ResponseWriter, r *http.Request) {
	s.session.Destroy(r)
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s server) addDefaultData(w http.ResponseWriter, r *http.Request, td *templateData) error {
	td.Authenticated = false
	td.User = nil
	td.CSRFTag = csrf.TemplateField(r)

	if s.session.Exists(r, "user_id") && s.session.GetBool(r, "authenticated") {
		userUuid := s.session.GetString(r, "user_id")
		var u user
		err := s.db.QueryRow("SELECT id, name, email, phone_number FROM user WHERE id = ?", userUuid).Scan(
			&u.Id,
			&u.Name,
			&u.Email,
			&u.PhoneNumber,
		)

		if err != nil {
			log.Println(err)
			return err
		}

		linkRows, err := s.db.Query("SELECT id, user_id, sort_order, url, display_url, screenshot_url FROM link where user_id = ?", userUuid)
		if err != nil {
			log.Println(err)
			return err
		}

		defer linkRows.Close()

		var links []link
		for linkRows.Next() {
			var l link
			err = linkRows.Scan(&l.Id, &l.UserId, &l.SortOrder, &l.Url, &l.DisplayUrl, &l.ScreenshotUrl)
			if err != nil {
				log.Println(err)
				return err
			}

			links = append(links, l)
		}

		err = linkRows.Err()
		if err != nil {
			log.Println(err)
			return err
		}

		td.Authenticated = true
		td.User = &u
		td.Links = links

		return nil
	}

	return nil
}

func (s server) render(w http.ResponseWriter, r *http.Request, viewTemplate string, td *templateData) {
	files := []string{"./views/" + viewTemplate, "./views/base.layout.tmpl"}

	ts, err := template.New(filepath.Base(viewTemplate)).Funcs(template.FuncMap{"mod": func(i, j int) bool { return (i+1)%j == 0 }}).ParseFiles(files...)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if td == nil {
		td = &templateData{}
	}

	err = s.addDefaultData(w, r, td)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err = ts.Execute(w, td)
	if err != nil {
		log.Println(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
