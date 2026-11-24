# Fileserver Package

The `fileserver` package serves binary content (attachments, avatars) to the signed-in browser over native HTTP with range support for Safari video and audio playback ([RFC 9110 §14](https://www.rfc-editor.org/rfc/rfc9110#section-14)). The web app reads attachment metadata through Connect.

## Endpoints

```text
GET /file/attachments/:uid[/:filename]   # attachment binary
    ?thumbnail=true                      # JPEG thumbnail for supported image types
    ?motion=true                         # embedded motion-photo video clip
GET /file/users/:identifier/avatar       # user avatar (by username)
GET /file/users/:identifier/export       # authenticated user's own Memos export (HEAD = preflight)
```

## Authentication

Handled by `server/auth` (see [authenticator.go](../auth/authenticator.go) and [token.go](../auth/token.go) for the constants and token formats). The server reads the browser session JWT from the `Authorization: Bearer` header or authenticates using the browser refresh cookie. See `getCurrentUser` in [fileserver.go](fileserver.go).

## Authorization

Attachments linked to a memo are readable only by the memo's active creator, regardless of the memo's stored
visibility. This ownership check also governs thumbnail and motion-photo requests. An unlinked attachment is
readable only by the account that uploaded it. Old share URLs follow the same authentication and ownership checks.

The private instance requires a web session to read an avatar.

The export route requires the requested username to match the signed-in account. It streams the ZIP as `core/memoexport`
writes it, admits one export at a time (a second request gets `429`), and charges the caller's `archive_user` budget.
`HEAD` runs the same checks and answers with the headers alone. The archive itself is assembled by `server/api`, which
the server injects as `MemoExporter`; without it the route answers `404`.

## Serving behavior

- **Local attachments** are served with HTTP range and conditional request support. Files are read from the managed attachment directory.
- **External links** redirect to their recorded HTTP(S) URLs after the same authorization check.
- **Thumbnails** are generated at max 600px, cached in `{data_dir}/.thumbnail_cache/`, with a semaphore capping concurrent generation. Images with HDR/wide-gamut metadata are served as originals, since re-encoding would strip it.
- **Motion photos** have their embedded video extracted and cached in `{data_dir}/.motion_cache/`.
- **XSS prevention**: script-capable MIME types are rewritten to `application/octet-stream`, non-media files get `Content-Disposition: attachment`, and all responses carry `X-Content-Type-Options: nosniff` plus a restrictive `Content-Security-Policy`.
- **Caching**: attachments and their thumbnails use `private, no-store`. Avatars also use `private, no-store`.

## Testing

Unit tests live in [fileserver_test.go](fileserver_test.go). For manual verification, sign in using a browser, open an attachment and inspect its requests in the Network tab. Check that the response matches the attachment bytes and that audio or video range requests return partial content.
