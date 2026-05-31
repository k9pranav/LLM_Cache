#!/usr/bin/env bash
set -e

APP_IMAGE="llm-cache-gateway"
APP_CONTAINER="llm-cache-gateway-container"
REDIS_CONTAINER="redis-cache"
NETWORK="llm-cache-network"

echo "Starting LLM Cache setup..."

echo "Creating Docker network if it does not exist..."
docker network inspect "$NETWORK" >/dev/null 2>&1 || docker network create "$NETWORK"

echo "Starting Redis container..."

if docker ps -a --format '{{.Names}}' | grep -q "^${REDIS_CONTAINER}$"; then
  if docker ps --format '{{.Names}}' | grep -q "^${REDIS_CONTAINER}$"; then
    echo "Redis container is already running."
  else
    echo "Redis container exists but is stopped. Starting it..."
    docker start "$REDIS_CONTAINER"
  fi
else
  echo "Creating Redis container..."
  docker run --name "$REDIS_CONTAINER" \
    --network "$NETWORK" \
    -p 6379:6379 \
    -d redis:7-alpine
fi

echo "Removing old app container if it exists..."
if docker ps -a --format '{{.Names}}' | grep -q "^${APP_CONTAINER}$"; then
  docker rm -f "$APP_CONTAINER"
fi

echo "Building app image..."
docker build -t "$APP_IMAGE" .

echo "Starting app container..."
docker run --name "$APP_CONTAINER" \
  --network "$NETWORK" \
  -p 8080:8080 \
  "$APP_IMAGE"