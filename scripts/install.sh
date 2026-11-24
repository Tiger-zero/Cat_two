#!/bin/sh

# Keep the legacy download URL as a pointer to the current Docker deployment.
printf '%s\n' \
  'Deploy Cat_two with Docker Compose.' \
  '' \
  '1. Save the Compose file from https://github.com/Tiger-zero/Cat_two/blob/main/compose.yaml' \
  '2. In the directory containing compose.yaml, run docker compose up -d.' \
  '3. Open http://<server-address>:5230 on a trusted network to create the administrator account.' \
  '' \
  'See https://github.com/Tiger-zero/Cat_two#部署服务端 for deployment and NAS instructions.'

case "${1:-}" in
  -h|--help) exit 0 ;;
  *) exit 1 ;;
esac
