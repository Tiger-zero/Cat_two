# Cat_two web authentication

[AuthContext](../src/contexts/AuthContext.tsx) owns the signed-in identity and user settings. Components read the current identity through [useCurrentUser](../src/hooks/useCurrentUser.ts). React Query stores server data and receives the verified current-user and profile entries from AuthContext. Profile changes update both.

## Initialization and privacy settings

- [AppInitializer](../src/main.tsx) starts authentication and instance initialization concurrently. It waits for the identity and instance profile before rendering routes.
- Authentication reuses a valid access token or attempts to refresh the session cookie. It verifies the account through `GetCurrentUser` before publishing the identity. The session check can complete before the user settings request.
- Screens may start fetching data once the identity is verified. They wait for display-sensitive settings, including rules for sensitive tags, before showing note content. If the first settings load fails, the signed-in layout offers a retry. A later refresh failure can keep settings already loaded for that identity. [Route guards](../src/router/guards.tsx) · [Signed-in layout](../src/layouts/RootLayout.tsx) · [Settings initialization tests](../tests/auth-context-initialization.test.tsx)

## Requests and session expiry

- The app sends the short-lived access token in the Bearer header. The browser sends the HttpOnly refresh cookie to the refresh endpoint, which rotates the credential. The Connect transport refreshes before expiry or after an authentication failure and retries an eligible request once. [Browser transport](../src/connect.ts) · [Server session handling](../../server/api/auth_service_session.go)
- Browser tabs share tokens for the same session. Web Locks serialize refresh on supported origins. On LAN HTTP without Web Locks, the transport can reuse a new token already published by another tab. [Token storage and synchronization](../src/auth-state.ts) · [Refresh tests](../tests/session-refresh.test.ts)
- In Cat_two, a web session expires after 45 minutes without editing a note. The server deadline applies to requests and refreshes. The browser observes the deadline, shares it across tabs, and returns to sign-in when it expires. Activity such as browsing and background synchronization does not extend the deadline. [Browser idle monitor](../src/lib/session-idle.ts) · [Session storage](../../store/user_session.go)
- Successful sign-out and session end clear the active identity and the React Query cache. Results from an earlier initialization are ignored when the user session changes. [Auth provider](../src/contexts/AuthContext.tsx)

The browser calls Connect, `/internal/`, and `/file/` with its session credentials. External MCP clients use dedicated connection tokens with their own permissions and expiry. [Integration credentials](../../.agents/skills/cat-two-product-development/references/external-api.md#凭据和可见性)
