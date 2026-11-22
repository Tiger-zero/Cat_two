package api

import (
	"net/http"
	"net/url"

	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

const authBrowserOriginError = "browser origin is not allowed for this authentication request"

// rejectsAuthBrowserOrigin checks the two unauthenticated procedures that set
// a session cookie. Clients that do not send a browser Origin can still sign
// in and refresh. Same-origin Fetch Metadata supports reverse proxies that
// change the scheme or Host seen by the backend.
func rejectsAuthBrowserOrigin(r *http.Request, procedure, instanceURL string) bool {
	switch procedure {
	case apiconnect.AuthServiceSignInProcedure, apiconnect.AuthServiceRefreshTokenProcedure:
	default:
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return false
	}
	if len(origins) != 1 {
		return true
	}
	if httphandler.MatchesBrowserOrigin(r, instanceURL) {
		return false
	}
	if sites := r.Header.Values("Sec-Fetch-Site"); len(sites) != 1 || sites[0] != "same-origin" {
		return true
	}
	origin, err := url.Parse(origins[0])
	return err != nil || (origin.Scheme != "http" && origin.Scheme != "https") || origin.Hostname() == "" ||
		origin.User != nil || origin.Path != "" || origin.ForceQuery || origin.RawQuery != "" || origin.Fragment != ""
}
