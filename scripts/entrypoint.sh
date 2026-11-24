#!/usr/bin/env sh

# Prepare an empty data directory, then drop to a non-root user.
MEMOS_UID=${MEMOS_UID:-${PUID:-10001}}
MEMOS_GID=${MEMOS_GID:-${PGID:-10001}}
DATA_DIR=${MEMOS_DATA:-/var/opt/memos}

empty_data_directory() {
    [ -d "$DATA_DIR" ] || return 1
    for entry in "$DATA_DIR"/* "$DATA_DIR"/.[!.]* "$DATA_DIR"/..?*; do
        if [ -e "$entry" ] || [ -L "$entry" ]; then
            return 1
        fi
    done
    return 0
}

# MEMOS_ENTRYPOINT_SWITCHED marks that the privilege drop below has already run.
# su-exec preserves the environment, so the marker survives the re-exec. Without
# it, a target of UID 0 (e.g. MEMOS_UID=0, common under rootless Docker) would
# stay root after su-exec, re-enter this block, and loop forever.
if [ "$(id -u)" = "0" ] && [ -z "${MEMOS_ENTRYPOINT_SWITCHED:-}" ]; then
    if empty_data_directory; then
        chown "$MEMOS_UID:$MEMOS_GID" "$DATA_DIR" 2>/dev/null || true
    fi
    echo "memos: starting as UID:GID ${MEMOS_UID}:${MEMOS_GID}"
    export MEMOS_ENTRYPOINT_SWITCHED=1
    exec su-exec "$MEMOS_UID:$MEMOS_GID" "$0" "$@"
fi
unset MEMOS_ENTRYPOINT_SWITCHED

file_env() {
   var="$1"
   fileVar="${var}_FILE"

   val_var="$(printenv "$var")"
   val_fileVar="$(printenv "$fileVar")"

   if [ -n "$val_var" ] && [ -n "$val_fileVar" ]; then
      echo "error: both $var and $fileVar are set (but are exclusive)" >&2
      exit 1
   fi

   if [ -n "$val_var" ]; then
      val="$val_var"
   elif [ -n "$val_fileVar" ]; then
      if [ ! -r "$val_fileVar" ]; then
         echo "error: file '$val_fileVar' does not exist or is not readable" >&2
         exit 1
      fi
      val="$(cat "$val_fileVar")"
   fi

   export "$var"="$val"
   unset "$fileVar"
}

file_env "MEMOS_DSN"

exec "$@"
