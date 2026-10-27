package http

import (
	"embed"
	"io/fs"
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger"
)

// swagger.json is the single, hand-maintained API spec. Keep it in sync with
// the routes registered in NewHandler.
//
//go:embed resource/swagger/swagger.json
var swaggerFiles embed.FS

const SwaggerJSONRoute = "/docs/swagger.json"

// RegisterSwagger serves the spec at SwaggerJSONRoute and the Swagger UI
// under /docs/.
func RegisterSwagger(mux *http.ServeMux) {
	sub, err := fs.Sub(swaggerFiles, "resource/swagger")
	if err != nil {
		panic(err) // the embedded path is fixed at build time
	}
	mux.Handle("GET "+SwaggerJSONRoute, http.StripPrefix("/docs/", http.FileServer(http.FS(sub))))
	mux.Handle("GET /docs/", httpSwagger.Handler(httpSwagger.URL(SwaggerJSONRoute)))
	mux.Handle("GET /docs", http.RedirectHandler("/docs/", http.StatusMovedPermanently))
}
