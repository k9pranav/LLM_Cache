#!/usr/bin/env bash
set -euo pipefail

APP_CONTAINER="llm-cache-gateway-container"
REDIS_CONTAINER="redis-cache"
OLLAMA_CONTAINER="ollama-cache"

echo "Pausing LLM Cache containers..."

if docker ps --format '{{.Names}}' | grep -q "^${APP_CONTAINER}$"; then
  echo "Stopping app container..."
  docker stop "$APP_CONTAINER"
else
  echo "App container is not running."
fi

if docker ps --format '{{.Names}}' | grep -q "^${REDIS_CONTAINER}$"; then
  echo "Stopping Redis container..."
  docker stop "$REDIS_CONTAINER"
else
  echo "Redis container is not running."
fi

if docker ps --format '{{.Names}}' | grep -q "^${OLLAMA_CONTAINER}$"; then
  echo "Stopping Ollama container..."
  docker stop "$OLLAMA_CONTAINER"
else
  echo "Ollama container is not running."
fi

echo "Paused."