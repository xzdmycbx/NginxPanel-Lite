// Package web serves the embedded React SPA with a client-side-routing fallback.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:dist
var dist embed.FS

// SPAHandler serves static assets from the embedded build, falling back to
// index.html for client routes. API paths return a JSON 404.
func SPAHandler() gin.HandlerFunc {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))
	return func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		if strings.HasPrefix(reqPath, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"code": "not_found", "message": "接口不存在"})
			return
		}
		p := strings.TrimPrefix(reqPath, "/")
		if p == "" {
			p = "index.html"
		}
		// Unknown path without a file extension -> let the SPA router handle it.
		if _, statErr := fs.Stat(sub, p); statErr != nil {
			c.Request.URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}
