package profile

import (
	"net/url"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

// SQLiteDatabasePath resolves the database name part of a supported SQLite DSN.
func SQLiteDatabasePath(dsn string) (string, error) {
	if !strings.HasPrefix(dsn, "file:") {
		if query := strings.IndexByte(dsn, '?'); query > 0 {
			return dsn[:query], nil
		}
		return dsn, nil
	}
	uri, err := url.Parse(dsn)
	if err != nil {
		return "", errors.Wrap(err, "invalid SQLite file URI")
	}
	if uri.Query().Get("mode") == "memory" {
		return ":memory:", nil
	}
	if uri.Opaque != "" {
		return url.PathUnescape(uri.Opaque)
	}
	path := uri.Path
	if runtime.GOOS == "windows" && len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return path, nil
}
