package messagecapture

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/pkg/errors"
)

// NetworkConfiguration controls whether the Telegram poller uses a custom proxy.
// The zero Source retains the standard runtime environment proxy behavior.
type NetworkConfiguration struct {
	Enabled  bool
	ProxyURL string
	Source   Source
}

// NetworkUpdate updates the Telegram proxy. An empty URL keeps the saved value.
type NetworkUpdate struct {
	Enabled  bool   `json:"enabled"`
	ProxyURL string `json:"proxyUrl,omitempty"`
}

func (u NetworkUpdate) apply(saved *savedConfiguration) error {
	proxyURL := secretOrCurrent(u.ProxyURL, saved.network().ProxyURL)
	if u.Enabled || proxyURL != "" {
		parsed, err := validateProxyURL(proxyURL)
		if err != nil {
			return err
		}
		proxyURL = parsed.String()
	}
	saved.Network = &NetworkUpdate{Enabled: u.Enabled, ProxyURL: proxyURL}
	return nil
}

func validateProxyURL(value string) (*url.URL, error) {
	invalid := errors.Wrap(ErrInvalidConfiguration, "enter a valid HTTP, HTTPS, SOCKS5 or SOCKS5H proxy URL with a host and optional port and credentials")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || strings.ContainsFunc(parsed.Hostname(), isSpaceOrControl) ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" || parsed.ForceQuery || parsed.RawQuery != "" ||
		strings.Contains(value, "#") {
		return nil, invalid
	}
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, invalid
	}
	if host := parsed.Hostname(); strings.HasPrefix(parsed.Host, "[") || strings.Contains(host, ":") {
		address, err := netip.ParseAddr(host)
		if err != nil || !address.Is6() || !strings.HasPrefix(parsed.Host, "[") {
			return nil, invalid
		}
	}
	if port := parsed.Port(); port != "" || strings.HasSuffix(parsed.Host, ":") {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return nil, invalid
		}
	}
	if parsed.User != nil {
		user := parsed.User.Username()
		password, _ := parsed.User.Password()
		if user == "" || strings.ContainsFunc(user, unicode.IsControl) || strings.ContainsFunc(password, unicode.IsControl) {
			return nil, invalid
		}
		if (parsed.Scheme == "socks5" || parsed.Scheme == "socks5h") && (len(user) > 255 || len(password) > 255) {
			return nil, invalid
		}
	}
	return parsed, nil
}

func isSpaceOrControl(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r)
}
