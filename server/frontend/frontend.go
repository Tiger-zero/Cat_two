package frontend

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/pkg/errors"

	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/profile"
)

//go:embed dist/*
var embeddedFiles embed.FS

const (
	frontendHTMLCacheControl        = "no-cache, no-store, must-revalidate"
	frontendStaticAssetCacheControl = "public, max-age=3600"
	frontendHashedAssetCacheControl = "public, max-age=2592000, immutable"
)

type FrontendService struct {
	Profile           *profile.Profile
	ChallengeProvider string
}

func NewFrontendService(profile *profile.Profile) *FrontendService {
	return &FrontendService{
		Profile: profile,
	}
}

// RegisterRetiredDiscoveryRoutes preserves the HTTP method contract for old public URLs.
func RegisterRetiredDiscoveryRoutes(mux *http.ServeMux) {
	handler := httphandler.GET(httphandler.Func(func(w http.ResponseWriter, _ *http.Request) error {
		w.Header().Set("Cache-Control", "no-store")
		return &httphandler.Error{Code: http.StatusNotFound, Message: "public instance discovery is unavailable"}
	}), nil)
	mux.Handle("/robots.txt", handler)
	mux.Handle("/sitemap.xml", handler)
}

// Handler serves static files and SPA routes, passing API namespaces to fallback.
func (s *FrontendService) Handler(fallback http.Handler) http.Handler {
	frontendFS := getFileSystem("dist")
	policy := s.contentSecurityPolicy()
	return httphandler.Func(func(w http.ResponseWriter, r *http.Request) error {
		requestPath := r.URL.Path
		if shouldSkipFrontendStatic(requestPath) {
			fallback.ServeHTTP(w, r)
			return nil
		}
		setFrontendCacheHeaders(w, requestPath)
		filename := strings.TrimPrefix(path.Clean("/"+requestPath), "/")
		if filename == "" {
			filename = "index.html"
		}
		info, err := fs.Stat(frontendFS, filename)
		if err == nil && info.IsDir() {
			filename = path.Join(filename, "index.html")
			_, err = fs.Stat(frontendFS, filename)
		}
		if err != nil {
			if !shouldServeFrontendHTML(requestPath) {
				w.Header().Set("Cache-Control", "no-store")
				return &httphandler.Error{Code: http.StatusNotFound, Message: http.StatusText(http.StatusNotFound)}
			}
			filename = "index.html"
		}
		// ServeContent avoids FileServer's /index.html and directory redirects.
		file, err := frontendFS.Open(filename)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err = file.Stat()
		if err != nil {
			return err
		}
		content, ok := file.(io.ReadSeeker)
		if !ok {
			return errors.New("frontend file does not support seeking")
		}
		if strings.HasSuffix(filename, ".html") {
			w.Header().Set("Content-Security-Policy", policy)
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "no-referrer")
		}
		http.ServeContent(w, r, info.Name(), info.ModTime(), content)
		return nil
	})
}

func (s *FrontendService) contentSecurityPolicy() string {
	const base = "frame-ancestors 'none'; object-src 'none'; base-uri 'self'"
	if s.Profile == nil || !s.Profile.Personal {
		return base
	}
	sources := "'self'"
	switch s.ChallengeProvider {
	case "turnstile":
		sources += " https://challenges.cloudflare.com"
	case "hcaptcha":
		sources += " https://hcaptcha.com https://*.hcaptcha.com"
	default:
		// An unconfigured or unrecognized provider adds no script sources.
	}
	// Drawing and image previews start blob workers supplied by the bundled canvas library.
	return base + "; script-src " + sources + "; script-src-attr 'none'; worker-src 'self' blob:"
}

func shouldSkipFrontendStatic(requestPath string) bool {
	if requestPath == "/robots.txt" || requestPath == "/sitemap.xml" {
		return true
	}
	return hasPathPrefix(requestPath, "/api") ||
		hasPathPrefix(requestPath, "/file") ||
		// RFC 8615 reserves this namespace for protocol metadata. The SPA
		// fallback already refuses it (see shouldServeFrontendHTML), but the
		// 404 that results would otherwise inherit the static asset cache
		// policy and be stored by clients and CDNs for an hour.
		hasPathPrefix(requestPath, "/.well-known") ||
		requestPath == "/memos.api" ||
		strings.HasPrefix(requestPath, "/memos.api.")
}

// SetCacheHeaders applies the frontend cache policy before route dispatch.
// Dynamic namespaces and discovery routes manage their own cache headers.
func SetCacheHeaders(w http.ResponseWriter, r *http.Request) {
	if !shouldSkipFrontendStatic(r.URL.Path) {
		setFrontendCacheHeaders(w, r.URL.Path)
	}
}

func setFrontendCacheHeaders(w http.ResponseWriter, requestPath string) {
	if shouldServeFrontendHTML(requestPath) {
		w.Header().Set("Cache-Control", frontendHTMLCacheControl)
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		return
	}

	cacheControl := frontendStaticAssetCacheControl
	if strings.HasPrefix(requestPath, "/assets/") {
		cacheControl = frontendHashedAssetCacheControl
	}
	w.Header().Set("Cache-Control", cacheControl)
}

// shouldServeFrontendHTML reports whether an unknown path is an SPA route
// that loads index.html. Paths with a file extension are assets, and paths
// with a hidden segment are never app routes: no username, memo UID, or fixed
// route begins with a dot. This keeps the SPA out of "/.well-known/", which
// RFC 8615 reserves for registered metadata that clients probe and expect a
// 404 for when the server does not offer it, and out of probes like "/.git/".
func shouldServeFrontendHTML(requestPath string) bool {
	if requestPath == "/" || requestPath == "/index.html" {
		return true
	}
	return path.Ext(requestPath) == "" && !strings.Contains(requestPath, "/.")
}

func hasPathPrefix(requestPath, prefix string) bool {
	return requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/")
}

func getFileSystem(path string) fs.FS {
	sub, err := fs.Sub(embeddedFiles, path)
	if err != nil {
		panic(err)
	}
	return sub
}
