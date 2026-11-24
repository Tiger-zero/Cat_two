package frontend

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/profile"
)

func TestFrontendService_CacheHeaderRules(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		cacheControl string
		pragma       string
		expires      string
	}{
		{
			name:         "root html is not stored",
			path:         "/",
			cacheControl: frontendHTMLCacheControl,
			pragma:       "no-cache",
			expires:      "0",
		},
		{
			name:         "index html is not stored",
			path:         "/index.html",
			cacheControl: frontendHTMLCacheControl,
			pragma:       "no-cache",
			expires:      "0",
		},
		{
			name:         "spa fallback html is not stored",
			path:         "/memos/publicmemo",
			cacheControl: frontendHTMLCacheControl,
			pragma:       "no-cache",
			expires:      "0",
		},
		{
			name:         "hashed asset is immutable",
			path:         "/assets/index-deadbeef.js",
			cacheControl: frontendHashedAssetCacheControl,
		},
		{
			name:         "stable root asset is revalidated after max age",
			path:         "/logo.webp",
			cacheControl: frontendStaticAssetCacheControl,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			setFrontendCacheHeaders(rec, tt.path)

			require.Equal(t, tt.cacheControl, rec.Header().Get("Cache-Control"))
			require.Equal(t, tt.pragma, rec.Header().Get("Pragma"))
			require.Equal(t, tt.expires, rec.Header().Get("Expires"))
		})
	}
}

func TestFrontendService_StaticCacheHeaders(t *testing.T) {
	e := http.NewServeMux()
	e.Handle("/", NewFrontendService(&profile.Profile{}).Handler(http.HandlerFunc(httphandler.NotFound)))

	tests := []struct {
		name         string
		path         string
		cacheControl string
		pragma       string
		expires      string
	}{
		{
			name:         "root html is not stored",
			path:         "/",
			cacheControl: frontendHTMLCacheControl,
			pragma:       "no-cache",
			expires:      "0",
		},
		{
			name:         "index html is not stored",
			path:         "/index.html",
			cacheControl: frontendHTMLCacheControl,
			pragma:       "no-cache",
			expires:      "0",
		},
		{
			name:         "spa fallback html is not stored",
			path:         "/memos/publicmemo",
			cacheControl: frontendHTMLCacheControl,
			pragma:       "no-cache",
			expires:      "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, tt.cacheControl, rec.Header().Get("Cache-Control"))
			require.Equal(t, tt.pragma, rec.Header().Get("Pragma"))
			require.Equal(t, tt.expires, rec.Header().Get("Expires"))
		})
	}
}

func TestFrontendService_SecurityHeaders(t *testing.T) {
	frontend := NewFrontendService(&profile.Profile{}).Handler(http.HandlerFunc(httphandler.NotFound))
	for _, path := range []string{"/", "/index.html", "/memos/private-note"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			frontend.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
			require.Equal(t, "DENY", recorder.Header().Get("X-Frame-Options"))
			require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
			require.Equal(t, "no-referrer", recorder.Header().Get("Referrer-Policy"))
		})
	}
}

func TestFrontendService_PersonalScriptPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, provider, scriptSource string
	}{
		{"stock server", "", "'self'"},
		{"Turnstile", "turnstile", "'self' https://challenges.cloudflare.com"},
		{"hCaptcha", "hcaptcha", "'self' https://hcaptcha.com https://*.hcaptcha.com"},
		{"unrecognized provider", "https://custom.example.test", "'self'"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := NewFrontendService(&profile.Profile{Personal: true})
			service.ChallengeProvider = tc.provider
			handler := service.Handler(http.HandlerFunc(httphandler.NotFound))
			for _, path := range []string{"/auth", "/auth/signup", "/home"} {
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
				require.Equal(t, http.StatusOK, rec.Code)
				policy := rec.Header().Get("Content-Security-Policy")
				require.Contains(t, policy, "script-src "+tc.scriptSource+";")
				require.Contains(t, policy, "script-src-attr 'none'")
				require.Contains(t, policy, "worker-src 'self' blob:")
				require.NotContains(t, policy, "'unsafe-inline'")
				require.NotContains(t, policy, "'unsafe-eval'")
			}
		})
	}
}

func TestFrontendService_MissingAssetsAreNotCached(t *testing.T) {
	e := http.NewServeMux()
	e.Handle("/", NewFrontendService(&profile.Profile{}).Handler(http.HandlerFunc(httphandler.NotFound)))

	for _, requestPath := range []string{"/assets/missing-deadbeef.js", "/assets/missing-deadbeef.css", "/missing.png"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+" "+requestPath, func(t *testing.T) {
				req := httptest.NewRequest(method, requestPath, nil)
				rec := httptest.NewRecorder()
				SetCacheHeaders(rec, req)
				e.ServeHTTP(rec, req)

				require.Equal(t, http.StatusNotFound, rec.Code)
				require.Equal(t, "no-store", rec.Result().Header.Get("Cache-Control"))
				require.NotContains(t, rec.Header().Get("Content-Type"), "text/html")
				if method == http.MethodHead {
					require.Empty(t, rec.Body.String())
				}
			})
		}
	}
}

func TestFrontendService_HiddenPathsDoNotFallbackToIndex(t *testing.T) {
	e := http.NewServeMux()
	e.Handle("/", NewFrontendService(&profile.Profile{}).Handler(http.HandlerFunc(httphandler.NotFound)))

	// OAuth Protected Resource Metadata discovery (RFC 9728) probes these
	// paths and reads a 404 as "not advertised"; HTML would break the client.
	for _, requestPath := range []string{
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-authorization-server",
		"/.git/config",
		"/u/.hidden",
	} {
		req := httptest.NewRequest(http.MethodGet, requestPath, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code, requestPath)
		require.NotContains(t, rec.Header().Get("Content-Type"), "text/html", requestPath)
	}

	req := httptest.NewRequest(http.MethodGet, "/memos/abc", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
}

// TestFrontendService_DiscoveryRoutesSkipFrontendCacheHeaders pins the cache
// policy for well-known URIs. The SPA fallback already refuses them, but the
// resulting 404 used to inherit the static asset policy, so clients and CDNs
// could store an absent discovery endpoint for an hour.
func TestFrontendService_DiscoveryRoutesSkipFrontendCacheHeaders(t *testing.T) {
	e := http.NewServeMux()
	e.Handle("/", NewFrontendService(&profile.Profile{}).Handler(http.HandlerFunc(httphandler.NotFound)))

	for _, requestPath := range []string{
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-authorization-server",
		"/.well-known/security.txt",
	} {
		req := httptest.NewRequest(http.MethodGet, requestPath, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code, requestPath)
		require.Empty(t, rec.Header().Get("Cache-Control"), requestPath)
		require.Empty(t, rec.Header().Get("Pragma"), requestPath)
		require.Empty(t, rec.Header().Get("Expires"), requestPath)
	}

	// SPA routes keep the frontend HTML policy, so the exclusion stays narrow.
	req := httptest.NewRequest(http.MethodGet, "/memos/abc", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, frontendHTMLCacheControl, rec.Header().Get("Cache-Control"))
}

func TestFrontendService_SkipsDynamicRoutes(t *testing.T) {
	e := http.NewServeMux()
	e.Handle("/", NewFrontendService(&profile.Profile{}).Handler(http.HandlerFunc(httphandler.NotFound)))
	e.Handle("GET /api/test", httphandler.Func(func(w http.ResponseWriter, _ *http.Request) error {
		return httphandler.Text(w, http.StatusOK, "ok")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Get("Cache-Control"))
}

func TestFrontendService_RetiredPublicDiscovery(t *testing.T) {
	handler := http.NewServeMux()
	RegisterRetiredDiscoveryRoutes(handler)
	for _, requestPath := range []string{"/robots.txt", "/sitemap.xml"} {
		req := httptest.NewRequest(http.MethodGet, requestPath, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNotFound, rec.Code)
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		require.Contains(t, rec.Body.String(), "public instance discovery is unavailable")
		require.NotContains(t, rec.Body.String(), "example.test")
	}
}
