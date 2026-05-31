package main

import (
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func (s server) getRouter() *echo.Echo {
	e := echo.New()

	e.Use(echo.WrapMiddleware(s.sessionManager.LoadAndSave))
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup: "form:_csrf",
	}))
	e.Use(middleware.RequestID())
	e.Use(middleware.TimeoutWithConfig(middleware.TimeoutConfig{
		Timeout: 60 * time.Second,
	}))

	e.Static("/static", "static")

	e.GET("/", s.home)
	e.POST("/link", s.createLink)
	e.GET("/sign-up", s.signUpForm)
	e.POST("/sign-up", s.signUp)
	e.GET("/log-in", s.logInForm)
	e.POST("/log-in", s.logIn)
	e.GET("/log-in/mfa-delivery-method", s.logInMfaDeliveryMethodForm)
	e.POST("/log-in/mfa-delivery-method", s.logInMfaDeliveryMethod)
	e.GET("/log-in/mfa", s.logInMfaForm)
	e.POST("/log-in/mfa", s.logInMfa)
	e.GET("/log-out", s.logOut)

	return e
}
