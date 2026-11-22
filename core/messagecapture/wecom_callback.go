package messagecapture

import (
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// WeComCallbackPath is the public route for signature-verified WeCom requests.
const WeComCallbackPath = "/integrations/wecom"

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// NormalizeWeComCallbackURL converts a DNS name or HTTP(S) URL to a callback URL.
// Bare domains use HTTPS. An empty value clears an override.
func NormalizeWeComCallbackURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	invalid := errors.Wrap(ErrInvalidConfiguration, "enter a public domain or an HTTP or HTTPS WeCom callback URL")
	if strings.ContainsAny(value, "?#") {
		return "", invalid
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil ||
		parsed.RawPath != "" || strings.HasPrefix(parsed.Host, "[") {
		return "", invalid
	}
	callbackPath := parsed.Path
	if callbackPath == "" || callbackPath == "/" {
		callbackPath = WeComCallbackPath
	}
	if !strings.HasSuffix(callbackPath, WeComCallbackPath) || path.Clean(callbackPath) != callbackPath {
		return "", invalid
	}
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if !isPublicDNSName(hostname) {
		return "", invalid
	}
	host := hostname
	if port := parsed.Port(); port != "" || strings.HasSuffix(parsed.Host, ":") {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return "", invalid
		}
		host += ":" + strconv.FormatUint(number, 10)
	}
	return (&url.URL{Scheme: parsed.Scheme, Host: host, Path: callbackPath}).String(), nil
}

func isPublicDNSName(hostname string) bool {
	if len(hostname) > 253 || strings.HasSuffix(hostname, ".home.arpa") || hostname == "home.arpa" {
		return false
	}
	labels := strings.Split(hostname, ".")
	if len(labels) < 2 {
		return false
	}
	domain := labels[len(labels)-1]
	if domain == "" || domain[0] < 'a' || domain[0] > 'z' {
		return false
	}
	switch domain {
	case "localhost", "local", "internal", "lan", "home", "corp":
		return false
	}
	for _, label := range labels {
		if !dnsLabel.MatchString(label) {
			return false
		}
	}
	return true
}
