package middleware

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// CORS allows the frontend (served from a different origin during local
// development, e.g. Vite on :5173) to call the API gateway. Origin is
// configurable via CORS_ALLOWED_ORIGIN; defaults to "*" for local dev.
func CORS() gin.HandlerFunc {
	allowedOrigin := os.Getenv("CORS_ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}

	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowedOrigin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
