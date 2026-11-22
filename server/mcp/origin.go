package mcp

import (
	"net/http"

	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/profile"
)

func isAllowedMCPOrigin(r *http.Request, profile *profile.Profile) bool {
	if r.Header.Get("Origin") == "" {
		return true
	}
	instanceURL := ""
	if profile != nil {
		instanceURL = profile.InstanceURL
	}
	return httphandler.MatchesBrowserOrigin(r, instanceURL)
}
