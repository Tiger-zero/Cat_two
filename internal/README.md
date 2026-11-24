# internal

Private plumbing with no memos vocabulary. A package belongs here only if it
could be published tomorrow as a standalone module without changes: it imports
nothing from `store`, `core`, `server`, `markdown`, `filter`, or
`proto/gen`. The `depguard` rules in `.golangci.yaml` enforce that.

| Package | Purpose |
| --- | --- |
| `clientip/` | trusted-proxy aware client IP resolution middleware |
| `httphandler/` | HTTP response errors, panic recovery, and routing compatibility |
| `identifier/` | grammar of UIDs and usernames shared by parser, store, and API |
| `linkmeta/` | HTML metadata, oEmbed, and image fetching for link previews |
| `motionphoto/` | Live Photo and Motion Photo container parsing |
| `profile/` | process configuration parsed from flags and environment |
| `random/` | UUIDs and random strings from a secure source |
| `ratelimit/` | sliding-window rate limiter |
| `testutil/` | image and motion-photo test fixtures (the one package allowed to import `proto/gen`) |
| `version/` | CalVer build metadata from release overrides or the embedded Git commit |

Do not add `util`, `common`, `base`, or `helpers` packages. Name a package for
the one thing it does, or put the code next to its only caller.
