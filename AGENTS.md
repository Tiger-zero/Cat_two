# AGENTS.md

Repository instructions for AI coding agents. Keep this file short, concrete, and tied to commands that actually work in this
repo. If a fact here conflicts with source files or CI config, trust the source file and update this guide.

## Project Snapshot

Cat_two is a self-hosted personal note-taking app based on Memos.

- Backend: Go 1.27.4, net/http, Connect RPC, gRPC-Gateway, Protocol Buffers.
- Frontend: React 19, TypeScript 7, Vite 8, Tailwind CSS v4, `@usememos/ui` on Base UI, React Query v5.
- Storage: SQLite is the only supported database for Cat_two deployments, migrations, compatibility checks, and tests.
- Generated API outputs: `proto/gen/` for Go/OpenAPI, `web/src/types/proto/` for TypeScript.

## Working Rules

- Read relevant code before editing; prefer local patterns over new abstractions.
- Keep diffs scoped. Do not do repo-wide cleanup, dependency churn, or generated-file rewrites unless the task requires it.
- Do not hand-edit generated proto outputs. Change `.proto` files, then run `buf generate`.
- For a schema change, add a SQLite migration and update `store/migration/sqlite/LATEST.sql`.
- Add public API endpoints to `server/api/acl_config.go`.
- Ask before adding heavy dependencies, changing auth/token behavior, or altering Docker/release workflows.
- Start repository work with the [project development skill](.agents/skills/cat-two-development/SKILL.md).
  Use the [product development skill](.agents/skills/cat-two-product-development/SKILL.md) for feature and API work,
  and the [UI development skill](.agents/skills/cat-two-ui-development/SKILL.md) for desktop design and frontend work.
  Use the [MCP usage skill](.agents/skills/cat-two-mcp/SKILL.md) when connecting to a deployed instance and organizing notes.

## Documentation

- ADRs and their companion glossary have been retired. Do not recreate them unless explicitly requested.
- Keep active design documents in step with their implementation until a tagged release ships the feature. After release, freeze the body and
  record later design changes in a dated `## Amendments` section. See the [design archive](.agents/skills/cat-two-development/references/design/README.md).
  Verify current behavior in source code and tests.
- Keep the [deployment README](README.md) and the relevant project, product or UI skill current. Maintain the work log in the
  [project development skill](.agents/skills/cat-two-development/SKILL.md#开发记录).
- Keep contributor guidance in this file and package-local `README.md` or `doc.go` files current when relevant code changes.

## Commands

Run from the repository root unless a command starts with `cd`.

```bash
# Backend
go run -buildvcs=true ./cmd/memos --port 8081 # Start backend with commit-derived CalVer
go run -buildvcs=true ./cmd/memos config plan --data DIR --config-dir DIR # Declarative configuration: also validate, apply, state
go test ./store/test -run '^TestMemoStore$' # Example of a focused SQLite test run
go build ./...                     # Compile all Go packages
go test ./...                      # Run the full Go suite
go test -v ./store/...             # SQLite store and migration tests
go test -v -race ./server/...      # Server tests with race detector
go test -v -race ./internal/...    # Internal package tests with race detector
go test -v -run TestFoo ./core/... # Run matching Go tests in one tree
go mod tidy -go=1.27.4             # Match CI tidy check
go fix ./...                      # Apply Go modernization fixes
go fix -diff ./...                 # Match CI modernization check
golangci-lint run                  # Go lint, config: .golangci.yaml (includes depguard layering rules)
golangci-lint run --fix            # Auto-fix lint, including goimports

# Frontend
cd web && pnpm install --frozen-lockfile # Use Node >=24.22.3 <25 and pnpm from web/package.json
cd web && pnpm dev                 # Dev server on :3001, proxying API to :8081 (or $DEV_PROXY_SERVER)
cd web && pnpm lint                # Type check + Biome lint
cd web && pnpm test                # Vitest unit tests
cd web && pnpm build               # Production build
cd web && pnpm release             # Build SPA into server/frontend/dist

# Protocol Buffers
cd proto && buf generate           # Regenerate Go + TypeScript + OpenAPI
cd proto && buf lint               # Lint proto files
cd proto && buf format -w          # Format proto files
```

## Repository Layout

Every root folder is named for what it holds. Read the folder's `doc.go` or `README.md` before adding to it.

| Path | Holds | May import |
| --- | --- | --- |
| `cmd/memos/` | Cobra/Viper CLI setup and server startup | anything |
| `server/` | The HTTP process: net/http bootstrap, graceful shutdown, every transport | core, store, markdown, filter, internal |
| `server/api/` | Connect/gRPC-Gateway services, ACL config, SSE hub. Thin handlers; rules go to `core/` | |
| `server/fileserver/` | Native HTTP file serving, thumbnails, range requests | |
| `server/frontend/` | Static SPA serving; `dist/` is the built SPA baked in by `go:embed` | |
| `server/mcp/` | Model Context Protocol server | |
| `server/auth/` | JWT access tokens, refresh tokens, PAT handling | |
| `core/` | Business rules for author access, memo payloads and exports, instance settings, message capture, MCP connections, users and declarative configuration | store, markdown, filter, internal |
| `store/` | Store facade, cache, SQLite migrations, and the `Driver` implemented in `store/db/sqlite/` | markdown, filter, internal |
| `markdown/` | Markdown engine: parser, AST, memos syntax extensions, renderer, memo payload | proto/gen, internal |
| `filter/` | CEL filter compiler, SQLite rendering, and filterable field schema | internal |
| `internal/` | Private plumbing for identifiers, trusted proxies, rate limits and HTTP helpers | third party only |
| `proto/api/`, `proto/store/` | Public API and internal storage proto sources; `proto/gen/` is generated | |
| `web/src/` | React SPA: `connect.ts` clients, `auth-state.ts`, `hooks/`, `contexts/`, `components/`, `lib/`, `locales/`, `themes/` | |

Layering rules, enforced by `depguard` in `.golangci.yaml`:

- Imports point downward only: `cmd` → `server` → `core` → `store` → {`markdown`, `filter`} → `internal`.
- `internal/` packages import nothing from this module (except `internal/testutil`, which may import `proto/gen`).
- A package goes in `internal/` only if it could be published as a standalone module unchanged. If it knows what a memo or instance setting is, it is a root package.
- Never create `util`, `common`, `base`, `helpers`, `misc`, or `pkg` packages. Name a package for the one thing it does, or put the code next to its only caller.
- New business rules extracted from `server/api` go to `core/<resource>/`, one package per resource.

File convention inside `server/api/`: `<resource>_service.go` holds the RPC handlers, `<resource>_service_converters.go` the
store↔proto conversion, and `<resource>_service_<topic>.go` further splits. Extend the matching file; do not add `*_helpers2.go`.
Declarative instance settings use API messages. Their store conversion lives in `core/instancesetting`; API response redaction lives in `server/api`.
Black-box service tests live in `server/api/test/`, unit tests next to the code.

## Change Routing

| Change | Update | Verify |
| --- | --- | --- |
| Go service or router behavior | Handlers under `server/`, rules under `core/`, tests near package | Go tests for affected packages and the affected HTTP, Connect, or MCP path |
| Store or migration behavior | `store/` and the SQLite migration or schema as applicable | Focused SQLite tests, then `go test ./store/...` for shared storage or schema changes |
| Markdown or filter logic | `markdown/`, `filter/` | Go tests for affected packages and integration paths |
| Internal package logic | Relevant `internal/` package tests | Go tests for affected packages |
| Frontend behavior | Components/hooks/contexts under `web/src/` | `cd web && pnpm lint && pnpm test` |
| Frontend production output | Vite config or release-sensitive UI | `cd web && pnpm build` or `pnpm release` |
| Proto API | `.proto` source, generated outputs, `proto/api/CHANGELOG.md` for breaks | `cd proto && buf generate && buf lint` |
| Public unauthenticated route | `server/api/acl_config.go` | Targeted server test or manual route check |

## Go Conventions

- Wrap errors with `errors.Wrap(err, "context")` from `github.com/pkg/errors`; do not use `fmt.Errorf`.
- Return service errors with `status.Errorf(codes.X, "message")`.
- Keep imports grouped as stdlib, third-party, then `github.com/usememos/memos`; goimports is run by golangci-lint.
- Add doc comments for exported identifiers; godot enforces exported comment punctuation.
- Avoid package-level mutable state unless the surrounding package already uses that pattern.

## Frontend Conventions

Use the [UI development skill](.agents/skills/cat-two-ui-development/SKILL.md) for the design system, editor, React Query,
localization, accessibility, and desktop verification. Read it before editing frontend code.

## Database And Proto Rules

- Keep SQLite fresh-install SQL and incremental migrations equivalent. Match both paths in storage tests. [Storage tests](store/test/README.md)
- Proto field changes must preserve compatibility unless the task explicitly allows a breaking API change. Record every
  allowed break in `proto/api/CHANGELOG.md`; compatibility is checked during review.
- Regenerate after proto edits and include both Go/OpenAPI and TypeScript generated outputs.

## Verification Policy

- Start with affected packages or named tests. Use SQLite for database checks and include race checks for concurrency and lifecycle changes.
- Before finishing, run the checks that match the changed surface from "Change Routing". Changes to shared behavior merit the broader Go suite and a full build.
- Before publishing, verify the release commit through the full CI gate and the SQLite container smoke checks.
- For docs-only changes, `git diff --check` is sufficient unless the docs include runnable examples that should be tested.
- If a required check cannot run locally, report the reason and the exact command that remains.

## CI Reference

- The [Cat_two Container workflow](.github/workflows/cat-two-container.yml) publishes numbered Docker Hub tags and
  `latest`. See the [development skill](.agents/skills/cat-two-development/SKILL.md) for publication and verification.
- Development builds use the HEAD committer date in UTC as `YY.MM.D-dev.<sha7>`. Docker Hub uses a separate image tag.
- `go build` embeds Git metadata automatically; `go run` needs `-buildvcs=true`. Builds without Git metadata must inject
  `github.com/usememos/memos/internal/version.Version` through `-ldflags`.
- Docker excludes `.git`; pass `--build-arg VERSION="$(bash scripts/release_version.sh development-version)"` and
  `--build-arg COMMIT="$(git rev-parse HEAD)"` when building locally (build frontend assets first).

- Workflows install toolchains through `.github/actions/setup-go` (Go version from `go.mod`) and `.github/actions/setup-web`
  (pnpm from `web/package.json`, Node 24, `pnpm install`); change versions there, not per workflow.
- Backend CI: Go 1.27.4, `go mod tidy`, `go fix -diff ./...`, golangci-lint v2.14.0 (including goimports), test groups
  `store`, `server`, `internal`, `other` (`cmd`, `core`, `markdown`, `filter`, `proto` and Cat_two script tests).
- Backend, frontend and Proto workflows use changed-path filters. The container workflow checks its input paths on ordinary
  pushes and always builds for manual or explicit publish requests.
- Backend store tests use SQLite, including fresh install and CalVer upgrade fixtures. To test a Cat_two image upgrade, pass
  `--previous-image` to the [container smoke script](scripts/release_smoke_test.sh).
- Frontend CI: Node 24, pnpm 11.0.1, `pnpm lint` and `pnpm test`. The container workflow builds the production web assets.
- Proto CI: `buf lint` and `buf format` check.
- Docker runtime uses Alpine 3.24 and runs the application as a non-root user on port 5230. Cat_two publishes amd64 and arm64. See `scripts/Dockerfile`.
