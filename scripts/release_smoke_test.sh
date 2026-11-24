#!/usr/bin/env bash

# Black-box smoke test for a Cat_two image.
#
# By default, the script builds the current worktree and checks a fresh install.
# Pass --candidate-image to test an image that has already been built.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

candidate_image="${MEMOS_SMOKE_CANDIDATE_IMAGE:-}"
expected_commit=""
platform=""
keep_resources="${MEMOS_SMOKE_KEEP_RESOURCES:-0}"

usage() {
  cat <<'EOF'
Usage: ./scripts/release_smoke_test.sh [options]

Verifies a fresh Cat_two install and data persistence after recreating its container.

Options:
  --candidate-image IMAGE  Test an existing image instead of building locally.
  --expected-commit SHA    Check the running candidate's complete commit SHA.
  --platform PLATFORM      Pull and run a specific platform, such as linux/arm64.
  --keep-resources         Keep containers and volumes after the test for debugging.
  -h, --help               Show this help text.

Environment equivalents:
  MEMOS_SMOKE_CANDIDATE_IMAGE
  MEMOS_SMOKE_KEEP_RESOURCES=1

Examples:
  ./scripts/release_smoke_test.sh
  ./scripts/release_smoke_test.sh --candidate-image cat-two:local
EOF
}

log() {
  printf '\n==> %s\n' "$*"
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

while (($# > 0)); do
  case "$1" in
    --candidate-image)
      (($# >= 2)) || die "--candidate-image requires a value"
      candidate_image="$2"
      shift 2
      ;;
    --expected-commit)
      (($# >= 2)) || die "--expected-commit requires a value"
      expected_commit="$2"
      shift 2
      ;;
    --platform)
      (($# >= 2)) || die "--platform requires a value"
      platform="$2"
      shift 2
      ;;
    --keep-resources)
      keep_resources=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "unknown option: $1"
      ;;
  esac
done

[[ -z "$expected_commit" || "$expected_commit" =~ ^[0-9a-f]{40}$ ]] || die "expected a complete Git commit SHA"
platform_option=()
if [[ -n "$platform" ]]; then
  case "$platform" in
    linux/amd64|linux/arm64) platform_option=(--platform "$platform") ;;
    *) die "unsupported smoke-test platform: $platform" ;;
  esac
fi

for command_name in cmp curl docker git jq; do
  command -v "$command_name" >/dev/null 2>&1 || die "$command_name is required"
done

docker info >/dev/null 2>&1 || die "Docker is not running"

run_id="${GITHUB_RUN_ID:-local}-$(date +%s)-$$"
fresh_container="memos-smoke-fresh-$run_id"
fresh_volume="memos-smoke-fresh-$run_id"
temp_dir="$(mktemp -d)"

container_exists() {
  docker container inspect "$1" >/dev/null 2>&1
}

cleanup() {
  local status=$?
  trap - EXIT

  if ((status != 0)) && container_exists "$fresh_container"; then
    printf '\n--- docker logs: %s ---\n' "$fresh_container" >&2
    docker logs "$fresh_container" >&2 || true
  fi

  if [[ "$keep_resources" == "1" ]]; then
    printf '\nKept smoke-test resources for debugging:\n'
    printf '  container: %s\n' "$fresh_container"
    printf '  volume:    %s\n' "$fresh_volume"
  else
    docker rm -f "$fresh_container" >/dev/null 2>&1 || true
    docker volume rm "$fresh_volume" >/dev/null 2>&1 || true
  fi

  rm -rf "$temp_dir"
  exit "$status"
}
trap cleanup EXIT

build_local_candidate() {
  local commit_sha build_version image_tag source_path build_context="$temp_dir/context"
  command -v pnpm >/dev/null 2>&1 || die "pnpm is required to build the local candidate"
  command -v tar >/dev/null 2>&1 || die "tar is required to build the local candidate"

  commit_sha="$(git -C "$REPO_ROOT" rev-parse HEAD)"
  build_version="$(bash "$SCRIPT_DIR/release_version.sh" development-version "$REPO_ROOT")"
  image_tag="memos-smoke:${commit_sha:0:12}-${run_id}"

  # Stage the current source, including local edits and new files, without taking
  # the existing embedded bundle or ignored caches into the Docker build context.
  mkdir -p "$build_context"
  git -C "$REPO_ROOT" ls-files --cached --others --exclude-standard -z \
    -- . ':(exclude)web' ':(exclude)server/frontend/dist' |
    while IFS= read -r -d '' source_path; do
      if [[ -f "$REPO_ROOT/$source_path" || -L "$REPO_ROOT/$source_path" ]]; then
        printf '%s\0' "$source_path"
      fi
    done |
    tar -cf - -C "$REPO_ROOT" --null -T - |
    tar -xf - -C "$build_context"

  log "Building frontend release assets"
  (
    cd "$REPO_ROOT/web"
    pnpm exec vite build --mode release --outDir "$build_context/server/frontend/dist" --emptyOutDir
  )

  log "Building candidate image $image_tag"
  docker build \
    --file "$build_context/scripts/Dockerfile" \
    "${platform_option[@]}" \
    --build-arg VERSION="$build_version" \
    --build-arg COMMIT="$commit_sha" \
    --tag "$image_tag" \
    "$build_context"

  candidate_image="$image_tag"
}

ensure_image() {
  local image="$1" cached_platform
  if cached_platform="$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$image" 2>/dev/null)" &&
    [[ -z "$platform" || "$cached_platform" == "$platform" ]]; then
    return
  fi
  log "Pulling image $image"
  docker pull "${platform_option[@]}" "$image"
}

wait_until_ready() {
  local container_name="$1"
  local base_url="$2"
  local attempt

  for attempt in $(seq 1 90); do
    if curl --fail --silent --show-error --max-time 2 "$base_url/healthz" >/dev/null 2>&1; then
      return
    fi
    if [[ "$(docker inspect --format '{{.State.Running}}' "$container_name" 2>/dev/null || true)" != "true" ]]; then
      break
    fi
    sleep 2
  done

  docker logs "$container_name" >&2 || true
  die "$container_name did not become ready"
}

current_base_url=""
session_cookies="$temp_dir/session.cookies"
: > "$session_cookies"

set_current_base_url() {
  local container_name="$1"
  local port_mapping port

  port_mapping="$(docker port "$container_name" 5230/tcp)"
  port="${port_mapping##*:}"
  [[ -n "$port" ]] || die "could not determine the host port for $container_name"
  current_base_url="http://127.0.0.1:$port"
}

# Use unary Connect JSON with a browser session.
connect_rpc() {
  local procedure="$1" payload="$2" token="${3:-}"
  local -a authorization=()
  if [[ -n "$token" ]]; then
    authorization=(--header "Authorization: Bearer $token")
  fi
  curl --fail-with-body --silent --show-error --max-time 30 \
    --header 'Content-Type: application/json' \
    --header 'Connect-Protocol-Version: 1' \
    --header "Origin: $current_base_url" \
    --cookie "$session_cookies" --cookie-jar "$session_cookies" \
    "${authorization[@]}" --data-binary "$payload" \
    "$current_base_url/memos.api.$procedure"
}

start_container() {
  local container_name="$1"
  local image="$2"
  local volume="$3"

  docker run --detach \
    "${platform_option[@]}" \
    --name "$container_name" \
    --label "org.usememos.release-smoke=$run_id" \
    --publish "127.0.0.1::5230" \
    --mount "type=volume,source=$volume,target=/var/opt/memos" \
    "$image" >/dev/null

  set_current_base_url "$container_name"
  wait_until_ready "$container_name" "$current_base_url"
}

assert_frontend_assets() {
  local html asset_path asset_url
  html="$(curl --fail --silent --show-error "$current_base_url/")"
  grep -q 'id="root"' <<<"$html" || die "frontend root element was not served"

  asset_path="$(grep -o 'src="[^"]*\.js"' <<<"$html" | sed -n '1{s/^src="//;s/"$//;p;}' || true)"
  [[ -n "$asset_path" ]] || die "frontend JavaScript asset was not found"
  case "$asset_path" in
    http://*|https://*) asset_url="$asset_path" ;;
    /*) asset_url="$current_base_url$asset_path" ;;
    *) asset_url="$current_base_url/$asset_path" ;;
  esac
  curl --fail --silent --show-error "$asset_url" >/dev/null
}

assert_candidate_commit() {
  if [[ -z "$expected_commit" ]]; then
    return
  fi
  connect_rpc 'InstanceService/GetInstanceProfile' '{}' |
    jq -e --arg commit "$expected_commit" '.commit == $commit' >/dev/null ||
    die "running candidate did not report commit $expected_commit"
}

create_admin() {
  local payload response
  payload='{"user":{"username":"smoke-admin","password":"smoke-password","email":"smoke@example.test"}}'
  response="$(connect_rpc 'UserService/CreateUser' "$payload")"
  jq -e '.username == "smoke-admin" and .role == "ADMIN"' <<<"$response" >/dev/null
}

sign_in() {
  local payload='{"passwordCredentials":{"username":"smoke-admin","password":"smoke-password"}}'
  connect_rpc 'AuthService/SignIn' "$payload" | jq -er '.accessToken | strings | select(length > 0)' >/dev/null || return
  # A cookie-only refresh exercises the Web session used after a page reload.
  refresh_session
}

refresh_session() {
  connect_rpc 'AuthService/RefreshToken' '{}' | jq -er '.accessToken | strings | select(length > 0)'
}

create_memo() {
  local token="$1"
  local memo_id="$2"
  local content="$3"
  local payload response
  payload="$(jq -nc --arg id "$memo_id" --arg content "$content" '{memoId:$id,memo:{content:$content,visibility:"PRIVATE"}}')"
  response="$(connect_rpc 'MemoService/CreateMemo' "$payload" "$token")"
  jq -e --arg name "memos/$memo_id" --arg content "$content" '.name == $name and .content == $content' <<<"$response" >/dev/null
}

assert_memo() {
  local token="$1" memo_id="$2" content="$3" payload
  payload="$(jq -nc --arg name "memos/$memo_id" '{name:$name}')"
  connect_rpc 'MemoService/GetMemo' "$payload" "$token" |
    jq -e --arg name "memos/$memo_id" --arg content "$content" '.name == $name and .content == $content' >/dev/null
}

trash_memo() {
  local token="$1" memo_id="$2" payload
  payload="$(jq -nc --arg name "memos/$memo_id" '{name:$name}')"
  connect_rpc 'MemoService/DeleteMemo' "$payload" "$token" >/dev/null
}

assert_restore_memo() {
  local token="$1" memo_id="$2" content="$3"
  curl --fail --silent --show-error --max-time 30 \
    --header "Authorization: Bearer $token" --header "Origin: $current_base_url" \
    "$current_base_url/internal/trash" |
    jq -e --arg name "memos/$memo_id" --arg content "$content" \
      '.memos | any(.memo.name == $name and .memo.content == $content)' >/dev/null
  curl --fail --silent --show-error --max-time 30 --request POST \
    --header "Authorization: Bearer $token" --header "Origin: $current_base_url" \
    "$current_base_url/internal/trash/$memo_id/restore" >/dev/null
  assert_memo "$token" "$memo_id" "$content"
}

assert_rest_retired() {
  local token="$1" path status
  for path in /api /api/instance/profile /api/memos /api/v1/memos /api/users/smoke-admin/personalAccessTokens; do
    status="$(curl --silent --show-error --max-time 30 --output /dev/null --write-out '%{http_code}' \
      --header "Authorization: Bearer $token" "$current_base_url$path")"
    [[ "$status" == "404" ]] || die "retired REST route $path returned HTTP $status, expected 404"
  done
}

attachment_filename="release-smoke.txt"
attachment_contents="Cat_two release smoke attachment"

create_attachment() {
  local token="$1" memo_id="$2" payload
  payload="$(jq -nc --arg filename "$attachment_filename" --arg contents "$attachment_contents" \
    --arg memo "memos/$memo_id" '{attachment:{filename:$filename,type:"text/plain",content:($contents|@base64),memo:$memo}}')"
  connect_rpc 'AttachmentService/CreateAttachment' "$payload" "$token" |
    jq -er '.name | select(test("^attachments/[A-Za-z0-9-]+$"))'
}

assert_attachment() {
  local token="$1" attachment="$2" memo_id="$3" payload
  payload="$(jq -nc --arg name "$attachment" '{name:$name}')"
  connect_rpc 'AttachmentService/GetAttachment' "$payload" "$token" |
    jq -e --arg memo "memos/$memo_id" --arg filename "$attachment_filename" \
      '.memo == $memo and .filename == $filename' >/dev/null
  curl --fail --silent --show-error --max-time 30 --header "Authorization: Bearer $token" \
    "$current_base_url/file/$attachment/$attachment_filename" --output "$temp_dir/attachment"
  printf '%s' "$attachment_contents" | cmp -s - "$temp_dir/attachment" || die "downloaded attachment bytes changed"
}

if [[ -z "$candidate_image" ]]; then
  build_local_candidate
else
  ensure_image "$candidate_image"
fi

log "Candidate image: $candidate_image"

log "Running fresh-install smoke test"
docker volume create "$fresh_volume" >/dev/null
start_container "$fresh_container" "$candidate_image" "$fresh_volume"
assert_candidate_commit
assert_frontend_assets
create_admin
fresh_token="$(sign_in)"
assert_rest_retired "$fresh_token"
create_memo "$fresh_token" "release-smoke" "fresh install smoke sentinel"
fresh_attachment="$(create_attachment "$fresh_token" "release-smoke")"
create_memo "$fresh_token" "release-smoke-trash" "trashed before volume reuse"
trash_memo "$fresh_token" "release-smoke-trash"

log "Recreating the candidate with the same SQLite volume"
docker rm -f "$fresh_container" >/dev/null
start_container "$fresh_container" "$candidate_image" "$fresh_volume"
assert_candidate_commit
fresh_token="$(refresh_session)"
assert_memo "$fresh_token" "release-smoke" "fresh install smoke sentinel"
assert_attachment "$fresh_token" "$fresh_attachment" "release-smoke"
assert_restore_memo "$fresh_token" "release-smoke-trash" "trashed before volume reuse"
create_memo "$fresh_token" "after-recreate-smoke" "created after container recreation"
assert_memo "$fresh_token" "after-recreate-smoke" "created after container recreation"

log "Fresh-install and volume persistence smoke tests passed"
