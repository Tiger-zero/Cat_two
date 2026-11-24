# Browser checks

On Linux, macOS or WSL, run the following from `web/` with the Node and Go versions configured by CI.

```sh
pnpm install --frozen-lockfile
pnpm exec playwright install chromium
pnpm exec playwright test --config tests/browser/app-smoke.config.ts
```

The app smoke test starts a fresh SQLite backend at `127.0.0.1:18081` and Vite at `127.0.0.1:3075`. Keep both ports free. It exercises browser sign-in, recovery of draft text and a staged attachment, saving the note, recycle-bin restoration, and authenticated download with byte comparison. It also creates a read-only MCP connection in Settings and uses the displayed address to discover tools and search for the restored note.

Set `CAT_TWO_CHROMIUM_EXECUTABLE` to an installed Chromium binary if Playwright's browser is unavailable in the environment.

## Check an existing split dev setup

Start the backend on an unused port and set `DEV_PROXY_SERVER` to its origin when starting Vite. Leave `MEMOS_INSTANCE_URL` unset for this check. Open **Settings → MCP** in the Vite page and create a test connection. The displayed MCP address should use the Vite origin and end in `/mcp`. Configure that address in a Streamable HTTP client, use the displayed Authorization value, then run tool discovery and a read such as `memo_search`. Delete the test connection afterwards.
