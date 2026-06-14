#!/usr/bin/env bash
set -euo pipefail

APP_IMAGE="llm-cache-gateway"
APP_CONTAINER="llm-cache-gateway-container"

REDIS_CONTAINER="redis-cache"
REDIS_IMAGE="redis/redis-stack-server:latest"

OLLAMA_CONTAINER="ollama-cache"
OLLAMA_IMAGE="ollama/ollama:latest"
OLLAMA_MODEL="mxbai-embed-large"

NETWORK="llm-cache-network"

echo "Starting LLM Cache setup..."

echo "Creating Docker network if it does not exist..."
docker network inspect "$NETWORK" >/dev/null 2>&1 || docker network create "$NETWORK"

echo "Starting Redis Stack container..."

if docker ps -a --format '{{.Names}}' | grep -q "^${REDIS_CONTAINER}$"; then
  CURRENT_REDIS_IMAGE="$(docker inspect -f '{{.Config.Image}}' "$REDIS_CONTAINER")"

  if [[ "$CURRENT_REDIS_IMAGE" != "$REDIS_IMAGE" ]]; then
    echo "Existing Redis container is not Redis Stack. Recreating it..."
    docker rm -f "$REDIS_CONTAINER"
  fi
fi

if docker ps -a --format '{{.Names}}' | grep -q "^${REDIS_CONTAINER}$"; then
  if docker ps --format '{{.Names}}' | grep -q "^${REDIS_CONTAINER}$"; then
    echo "Redis Stack container is already running."
  else
    echo "Redis Stack container exists but is stopped. Starting it..."
    docker start "$REDIS_CONTAINER"
  fi
else
  echo "Creating Redis Stack container..."
  docker run --name "$REDIS_CONTAINER" \
    --network "$NETWORK" \
    -p 6379:6379 \
    -d "$REDIS_IMAGE"
fi

echo "Starting Ollama container..."

if docker ps -a --format '{{.Names}}' | grep -q "^${OLLAMA_CONTAINER}$"; then
  if docker ps --format '{{.Names}}' | grep -q "^${OLLAMA_CONTAINER}$"; then
    echo "Ollama container is already running."
  else
    echo "Ollama container exists but is stopped. Starting it..."
    docker start "$OLLAMA_CONTAINER"
  fi
else
  echo "Creating Ollama container..."
  docker run --name "$OLLAMA_CONTAINER" \
    --network "$NETWORK" \
    -p 11434:11434 \
    -v ollama-data:/root/.ollama \
    -d "$OLLAMA_IMAGE"
fi

echo "Waiting for Ollama to be ready..."
until docker exec "$OLLAMA_CONTAINER" ollama list >/dev/null 2>&1; do
  sleep 2
done

echo "Pulling embedding model: $OLLAMA_MODEL"
docker exec "$OLLAMA_CONTAINER" ollama pull "$OLLAMA_MODEL"

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