import { resolve } from "node:path";
import { defineConfig } from "@playwright/test";

const repository = resolve(__dirname, "../../..");
const backend = "http://127.0.0.1:18081";
const baseURL = "http://127.0.0.1:3075";

export default defineConfig({
  testDir: ".",
  testMatch: "app-smoke.spec.ts",
  workers: 1,
  reporter: "list",
  timeout: 150_000,
  outputDir: "../../node_modules/.cache/app-smoke-results",
  use: {
    baseURL,
    browserName: "chromium",
    launchOptions: { executablePath: process.env.CAT_TWO_CHROMIUM_EXECUTABLE },
    viewport: { width: 1280, height: 900 },
    actionTimeout: 10_000,
    trace: "retain-on-failure",
  },
  webServer: [
    {
      cwd: repository,
      command: `set -eu
data=$(mktemp -d)
trap 'rm -rf -- "$data"' EXIT
trap 'exit 143' TERM
go run -buildvcs=true ./cmd/memos --addr 127.0.0.1 --port 18081 --unix-sock= --instance-url= \
  --driver sqlite --data "$data" --dsn "$data/smoke.db" --demo=false --config-apply=off`,
      url: `${backend}/healthz`,
      timeout: 180_000,
      reuseExistingServer: false,
      gracefulShutdown: { signal: "SIGTERM", timeout: 5_000 },
    },
    {
      cwd: resolve(repository, "web"),
      command: "pnpm exec vite --host 127.0.0.1 --port 3075 --strictPort",
      env: { DEV_PROXY_SERVER: backend },
      url: baseURL,
      timeout: 60_000,
      reuseExistingServer: false,
    },
  ],
});
