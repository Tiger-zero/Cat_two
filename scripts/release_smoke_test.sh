#!/usr/bin/env bash

# Black-box smoke test for a Cat_two image.
#
# By default, the script builds the current worktree and checks a fresh install.
# Pass --candidate-image to test an image that has already been built.

set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

candidate_image="${MEMOS_SMOKE_CANDIDATE_IMAGE:-}"
previous_image="${MEMOS_SMOKE_PREVIOUS_IMAGE:-}"
expected_commit=""
platform=""
keep_resources="${MEMOS_SMOKE_KEEP_RESOURCES:-0}"
fresh_only=0

usage() {
  cat <<'EOF'
Usage: ./scripts/release_smoke_test.sh [options]

Verifies a fresh Cat_two install and persistent data. Provide a previous
Cat_two image to also verify an upgrade.

Options:
  --candidate-image IMAGE  Test an existing image instead of building locally.
  --previous-image IMAGE   Cat_two image used to seed an optional upgrade test.
  --expected-commit SHA    Check the running candidate's complete commit SHA.
  --platform PLATFORM      Pull and run a specific platform, such as linux/arm64.
  --fresh-only             Run fresh install and persistence checks regardless of the previous image.
  --keep-resources         Keep containers and volumes after the test for debugging.
  -h, --help               Show this help text.

Environment equivalents:
  MEMOS_SMOKE_CANDIDATE_IMAGE
  MEMOS_SMOKE_PREVIOUS_IMAGE
  MEMOS_SMOKE_KEEP_RESOURCES=1

Examples:
  ./scripts/release_smoke_test.sh
  ./scripts/release_smoke_test.sh \
    --candidate-image cat-two:local \
    --previous-image docker.io/tiger2896/cat_two:0.06
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
    --previous-image)
      (($# >= 2)) || die "--previous-image requires a value"
      previous_image="$2"
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
    --fresh-only)
      fresh_only=1
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

if [[ -z "$previous_image" ]]; then
  fresh_only=1
fi

for command_name in cmp curl docker git jq; do
  command -v "$command_name" >/dev/null 2>&1 || die "$command_name is required"
done

docker info >/dev/null 2>&1 || die "Docker is not running"

run_id="${GITHUB_RUN_ID:-local}-$(date +%s)-$$"
fresh_container="memos-smoke-fresh-$run_id"
upgrade_old_container="memos-smoke-upgrade-old-$run_id"
upgrade_new_container="memos-smoke-upgrade-new-$run_id"
fresh_volume="memos-smoke-fresh-$run_id"
upgrade_volume="memos-smoke-upgrade-$run_id"
temp_dir="$(mktemp -d)"

container_exists() {
  docker container inspect "$1" >/dev/null 2>&1
}

cleanup() {
  local status=$?
  trap - EXIT

  if ((status != 0)); then
    for container_name in "$fresh_container" "$upgrade_old_container" "$upgrade_new_container"; do
      if container_exists "$container_name"; then
        printf '\n--- docker logs: %s ---\n' "$container_name" >&2
        docker logs "$container_name" >&2 || true
      fi
    done
  fi

  if [[ "$keep_resources" == "1" ]]; then
    printf '\nKept smoke-test resources for debugging:\n'
    printf '  containers: %s %s %s\n' "$fresh_container" "$upgrade_old_container" "$upgrade_new_container"
    printf '  volumes:    %s %s\n' "$fresh_volume" "$upgrade_volume"
  else
    docker rm -f "$fresh_container" "$upgrade_old_container" "$upgrade_new_container" >/dev/null 2>&1 || true
    docker volume rm "$fresh_volume" "$upgrade_volume" >/dev/null 2>&1 || true
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

set_current_base_url() {
  local container_name="$1"
  local port_mapping port

  port_mapping="$(docker port "$container_name" 5230/tcp)"
  port="${port_mapping##*:}"
  [[ -n "$port" ]] || die "could not determine the host port for $container_name"
  current_base_url="http://127.0.0.1:$port"
}

# api_url prints the API root of the running container. Older releases serve the
# API under /api/v1; current releases serve it under /api.
api_url() {
  if curl --fail --silent --output /dev/null --max-time 2 "$current_base_url/api/instance/profile"; then
    printf '%s/api\n' "$current_base_url"
  else
    printf '%s/api/v1\n' "$current_base_url"
  fi
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
    --env MEMOS_MODE=prod \
    --env MEMOS_INSTANCE_URL=http://localhost \
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
  curl --fail --silent --show-error "$(api_url)/instance/profile" |
    jq -e --arg commit "$expected_commit" '.commit == $commit' >/dev/null ||
    die "running candidate did not report commit $expected_commit"
}

create_admin() {
  local payload response
  payload="$(jq -nc '{username:"smoke-admin",password:"smoke-password",email:"smoke@example.test"}')"
  response="$(curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --data "$payload" \
    "$(api_url)/users")"
  jq -e '.username == "smoke-admin" and .role == "ADMIN"' <<<"$response" >/dev/null
}

sign_in() {
  local payload
  payload="$(jq -nc '{passwordCredentials:{username:"smoke-admin",password:"smoke-password"}}')"
  curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --data "$payload" \
    "$(api_url)/auth/signin" | jq -er '.accessToken'
}

create_memo() {
  local token="$1"
  local memo_id="$2"
  local content="$3"
  local payload response
  payload="$(jq -nc --arg content "$content" '{content:$content,visibility:"PRIVATE"}')"
  response="$(curl --fail --silent --show-error \
    --header "Authorization: Bearer $token" \
    --header 'Content-Type: application/json' \
    --data "$payload" \
    "$(api_url)/memos?memoId=$memo_id")"
  jq -e --arg name "memos/$memo_id" --arg content "$content" '.name == $name and .content == $content' <<<"$response" >/dev/null
}

assert_memo() {
  local token="$1"
  local memo_id="$2"
  local content="$3"
  curl --fail --silent --show-error \
    --header "Authorization: Bearer $token" \
    "$(api_url)/memos/$memo_id" |
    jq -e --arg content "$content" '.content == $content' >/dev/null
}

attachment_filename="release-smoke.txt"
attachment_contents="Cat_two release smoke attachment"

create_attachment() {
  local token="$1" memo_id="$2" payload
  payload="$(jq -nc --arg filename "$attachment_filename" --arg contents "$attachment_contents" \
    --arg memo "memos/$memo_id" '{filename:$filename,type:"text/plain",content:($contents|@base64),memo:$memo}')"
  curl --fail --silent --show-error \
    --header "Authorization: Bearer $token" \
    --header 'Content-Type: application/json' \
    --data "$payload" "$(api_url)/attachments" |
    jq -er '.name | select(test("^attachments/[A-Za-z0-9-]+$"))'
}

assert_attachment() {
  local token="$1" attachment="$2" memo_id="$3"
  curl --fail --silent --show-error --header "Authorization: Bearer $token" "$(api_url)/$attachment" |
    jq -e --arg memo "memos/$memo_id" --arg filename "$attachment_filename" \
      '.memo == $memo and .filename == $filename' >/dev/null
  curl --fail --silent --show-error --header "Authorization: Bearer $token" \
    "$current_base_url/file/$attachment/$attachment_filename" --output "$temp_dir/attachment"
  printf '%s' "$attachment_contents" | cmp -s - "$temp_dir/attachment" || die "downloaded attachment bytes changed"
}

if [[ -z "$candidate_image" ]]; then
  build_local_candidate
else
  ensure_image "$candidate_image"
fi

if [[ "$fresh_only" == "0" ]]; then
  ensure_image "$previous_image"
  [[ "$candidate_image" != "$previous_image" ]] || die "candidate and previous images must be different"
  log "Previous image:  $previous_image"
fi

log "Candidate image: $candidate_image"

log "Running fresh-install smoke test"
docker volume create "$fresh_volume" >/dev/null
start_container "$fresh_container" "$candidate_image" "$fresh_volume"
assert_candidate_commit
assert_frontend_assets
create_admin
fresh_token="$(sign_in)"
create_memo "$fresh_token" "release-smoke" "fresh install smoke sentinel"
fresh_attachment="$(create_attachment "$fresh_token" "release-smoke")"

docker rm -f "$fresh_container" >/dev/null
start_container "$fresh_container" "$candidate_image" "$fresh_volume"
fresh_token="$(sign_in)"
assert_memo "$fresh_token" "release-smoke" "fresh install smoke sentinel"
assert_attachment "$fresh_token" "$fresh_attachment" "release-smoke"
docker rm -f "$fresh_container" >/dev/null

if [[ "$fresh_only" == "1" ]]; then
  log "Fresh-install and volume persistence smoke tests passed"
  exit 0
fi

log "Running $previous_image to $candidate_image upgrade smoke test"
docker volume create "$upgrade_volume" >/dev/null
start_container "$upgrade_old_container" "$previous_image" "$upgrade_volume"
create_admin
upgrade_token="$(sign_in)"
create_memo "$upgrade_token" "pre-upgrade-smoke" "created before release upgrade"
upgrade_attachment="$(create_attachment "$upgrade_token" "pre-upgrade-smoke")"
create_memo "$upgrade_token" "pre-upgrade-trash" "trashed before release upgrade"
curl --fail --silent --show-error --request DELETE --header "Authorization: Bearer $upgrade_token" \
  "$(api_url)/memos/pre-upgrade-trash" >/dev/null
docker rm -f "$upgrade_old_container" >/dev/null

start_container "$upgrade_new_container" "$candidate_image" "$upgrade_volume"
assert_candidate_commit
assert_frontend_assets
upgrade_token="$(sign_in)"
assert_memo "$upgrade_token" "pre-upgrade-smoke" "created before release upgrade"
assert_attachment "$upgrade_token" "$upgrade_attachment" "pre-upgrade-smoke"
curl --fail --silent --show-error --header "Authorization: Bearer $upgrade_token" "$current_base_url/api/trash" |
  jq -e '.memos | any(.memo.name == "memos/pre-upgrade-trash" and .memo.content == "trashed before release upgrade")' >/dev/null
curl --fail --silent --show-error --request POST --header "Authorization: Bearer $upgrade_token" \
  "$current_base_url/api/trash/pre-upgrade-trash/restore" >/dev/null
assert_memo "$upgrade_token" "pre-upgrade-trash" "trashed before release upgrade"
create_memo "$upgrade_token" "post-upgrade-smoke" "created after release upgrade"
assert_memo "$upgrade_token" "post-upgrade-smoke" "created after release upgrade"

log "Release smoke tests passed"
