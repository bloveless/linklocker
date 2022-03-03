package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

func (s server) getRouter() *chi.Mux {
	r := chi.NewRouter()

	// A good base middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Set a timeout value on the request context (ctx), that will signal
	// through ctx.Done() that the request has timed out and further
	// processing should be stopped.
	r.Use(middleware.Timeout(60 * time.Second))

	fs := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fs))

	r.Group(func(r chi.Router) {
		r.Use(s.session.Enable)
		r.Get("/", s.home)
		r.Post("/link", s.createLink)
		r.Get("/sign-up", s.signUpForm)
		r.Post("/sign-up", s.signUp)
		r.Get("/log-in", s.logInForm)
		r.Post("/log-in", s.logIn)
		r.Get("/log-out", s.logOut)
	})

	return r
}
