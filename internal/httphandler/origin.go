package httphandler

import (
	"net/http"
	"net/url"
	"strings"
)

// MatchesBrowserOrigin compares the Origin header with the configured public
// origin or the connection's own origin. Proxy forwarding headers do not
// determine browser trust.
func MatchesBrowserOrigin(r *http.Request, instanceURL string) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || !isWebOrigin(origin) || origin.Path != "" || origin.ForceQuery || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	canonical, err := url.Parse(instanceURL)
	if err == nil && isWebOrigin(canonical) && strings.EqualFold(origin.Hostname(), canonical.Hostname()) {
		// For the public hostname, pin its scheme and port even when a reverse
		// proxy uses a plain HTTP connection to the app.
		return equalWebOrigin(origin, canonical)
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return equalWebOrigin(origin, &url.URL{Scheme: scheme, Host: r.Host})
}

func isWebOrigin(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil
}

func equalWebOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && strings.EqualFold(a.Hostname(), b.Hostname()) && originPort(a) == originPort(b)
}

func originPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}
