#!/usr/bin/env bash
# Build amd64 DocFlow images on the Mac, ship them to the VM, and start them there.
# The VM never builds. Run from anywhere in the repo: scripts/deploy-vm.sh
set -euo pipefail

VM=${VM:-project@10.198.200.62}
VM_DIR=${VM_DIR:-docflow/kku-order}

cd "$(dirname "$0")/.."

# Refuse uncommitted changes to tracked files so the tag matches the code.
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  echo "Tracked files have uncommitted changes. Commit first." >&2
  exit 1
fi

TAG=$(git rev-parse --short HEAD)
API_IMG=docflow-api:$TAG
WEB_IMG=docflow-web:$TAG

# Build with the same Dockerfiles and build args as docker-compose.yml.
echo "==> Building $API_IMG and $WEB_IMG (linux/amd64)"
docker buildx build --platform linux/amd64 --load -t "$API_IMG" ./backend
docker buildx build --platform linux/amd64 --load -t "$WEB_IMG" \
  --build-arg NEXT_PUBLIC_API_URL=/backend \
  --build-arg BACKEND_INTERNAL_URL=http://api:8080 .

# Stream both images to the VM without a temp file.
echo "==> Shipping images to $VM"
docker save "$API_IMG" "$WEB_IMG" | gzip | ssh "$VM" 'gunzip | docker load'

# Copy only the compose files. Never touch .env, .env.local or docker-compose.vm.yml.
echo "==> Copying compose files"
scp docker-compose.yml docker-compose.ocr-net.yml "$VM:~/$VM_DIR/"

# Point compose at the new tags, start api and web without building, and wait for api health.
echo "==> Starting on the VM"
# The script goes in as an argument, not stdin, so no remote command can eat it.
REMOTE_CMD=$(cat <<'REMOTE'
set -euo pipefail
TAG=$1
cd ~/"$2"

# Replace KEY=... in .env, or append it if missing.
set_env() {
  if grep -q "^$1=" .env; then
    sed -i "s|^$1=.*|$1=$2|" .env
  else
    echo "$1=$2" >> .env
  fi
}
set_env API_IMAGE "docflow-api:$TAG"
set_env WEB_IMAGE "docflow-web:$TAG"

docker compose up -d --no-build api web

# Poll the api healthcheck for up to 3 minutes.
deadline=$((SECONDS + 180))
status=""
while [ "$SECONDS" -lt "$deadline" ]; do
  status=$(docker inspect -f '{{.State.Health.Status}}' docflow-api 2>/dev/null || true)
  [ "$status" = healthy ] && break
  sleep 5
done

docker compose ps
if [ "$status" != healthy ]; then
  echo "api is not healthy (status: ${status:-unknown}). Last 30 api log lines:" >&2
  docker compose logs --tail=30 api >&2 || true
  exit 1
fi
REMOTE
)
ssh "$VM" "bash -c $(printf '%q' "$REMOTE_CMD") _ $(printf '%q ' "$TAG" "$VM_DIR")"

echo "==> Deployed $TAG (API_IMAGE=$API_IMG, WEB_IMAGE=$WEB_IMG) to $VM:$VM_DIR"
