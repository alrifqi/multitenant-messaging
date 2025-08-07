package middleware

import (
	"log"
	"net/http"
	"strings"

	"github.com/alrifqi/multitenant-messaging/internal/auth"
	"github.com/alrifqi/multitenant-messaging/internal/models"

	"github.com/labstack/echo/v4"
)

// JWTAuth middleware for JWT authentication
func JWTAuth(jwtManager *auth.JWTManager) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip authentication for certain endpoints
			if shouldSkipAuth(c.Path()) {
				return next(c)
			}

			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return c.JSON(http.StatusUnauthorized, models.APIResponse{
					Success: false,
					Error:   "Authorization header required",
				})
			}

			token, err := auth.ExtractTokenFromHeader(authHeader)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, models.APIResponse{
					Success: false,
					Error:   "Invalid authorization header format",
				})
			}

			claims, err := jwtManager.ValidateToken(token)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, models.APIResponse{
					Success: false,
					Error:   "Invalid or expired token",
				})
			}

			// Set user information in context
			c.Set("user_id", claims.UserID)
			c.Set("username", claims.Username)
			c.Set("tenant_id", claims.TenantID)

			return next(c)
		}
	}
}

// shouldSkipAuth checks if the endpoint should skip authentication
func shouldSkipAuth(path string) bool {
	skipPaths := []string{
		"/auth/login",
		"/health",
		"/metrics",
		"/swagger",
	}

	for _, skipPath := range skipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}
	return false
}

// Logging middleware for request logging
func Logging() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Log request
			req := c.Request()
			log.Printf("Request: %s %s", req.Method, req.URL.Path)

			// Process request
			err := next(c)

			// Log response
			res := c.Response()
			log.Printf("Response: %d %s", res.Status, http.StatusText(res.Status))

			return err
		}
	}
}

// CORS middleware for handling CORS
func CORS() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("Access-Control-Allow-Origin", "*")
			c.Response().Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Response().Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if c.Request().Method == "OPTIONS" {
				return c.NoContent(http.StatusNoContent)
			}

			return next(c)
		}
	}
}
