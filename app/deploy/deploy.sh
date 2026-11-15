#!/usr/bin/env bash
set -euo pipefail

APP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
docker compose -f "$APP_DIR/docker-compose.yml" up --build -d
docker compose -f "$APP_DIR/docker-compose.yml" ps
