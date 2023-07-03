package handlers

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"strings"

	_ "embed"

	chi "github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

//go:embed views
var viewsFS embed.FS

type renderedData struct {
	SiteKey        string
	BackendHostApi string
	ApiKey         string
}

func Render(mux chi.Router, log *zap.Logger) {
	log.Info("starting to render the index.html")

	// Template data
	renderedData := renderedData{
		SiteKey:        getEnvVarOrError("CAPTCHA_SITE_KEY"),
		BackendHostApi: getEnvVarOrError("BACKEND_HOST_API"),
		ApiKey:         getEnvVarOrError("API_KEY"),
	}

	// Path to build where assets are
	filesDir, err := fs.Sub(viewsFS, "views")
	if err != nil {
		panic(err)
	}

	// Index template
	view, err := template.ParseFS(viewsFS, "views/index.html")
	if err != nil {
		panic(err)
	}

	mux.Get("/", func(w http.ResponseWriter, r *http.Request) {
		view.Execute(w, renderedData)
	})
	FileServer(mux, "/", http.FS(filesDir))
}

func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", 301).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}

func getEnvVarOrError(name string) string {
	v, ok := os.LookupEnv(name)
	if !ok {
		panic(errors.New(fmt.Sprintf("given env var is not present, %s not found !", name)))
	}
	return v
}
