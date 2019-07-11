package main

import (
	"net/http"
	"strings"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

// KeyAuthConfig defines the config for CookieAuth middleware.
type CookieAuthConfig struct {
	Skipper middleware.Skipper
}

var (
	// DefaultCookieAuthConfig is the default CookieAuth middleware config.
	DefaultCookieAuthConfig = CookieAuthConfig{
		Skipper: middleware.DefaultSkipper,
	}
)

func staticSkipper(c echo.Context) bool {
	if c.Path() == "/login" ||
		c.Path() == "/logout" ||
		strings.HasPrefix(c.Path(), "/css/") ||
		strings.HasPrefix(c.Path(), "/js/") ||
		strings.HasPrefix(c.Path(), "/images/") {
		return true
	}

	return false
}

func Auth() echo.MiddlewareFunc {
	c := DefaultCookieAuthConfig
	c.Skipper = staticSkipper
	return authWithConfig(c)
}

func authWithConfig(conf CookieAuthConfig) echo.MiddlewareFunc {
	if conf.Skipper == nil {
		conf.Skipper = DefaultCookieAuthConfig.Skipper
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if conf.Skipper(c) {
				return next(c)
			}

			if !hasLogin {
				c.Redirect(http.StatusSeeOther, "/login")
				return nil
			}
			return next(c)
		}
	}
}
