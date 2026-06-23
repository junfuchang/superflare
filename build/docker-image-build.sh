#!/bin/bash
VERSION=$(git describe --tags --abbrev=0)
COMMIT=$(git rev-parse --short HEAD)

echo "最近版本；$VERSION / $COMMIT"
echo "$VERSION">RELEASE_VERSION
echo "$COMMIT">RELEASE_COMMIT

DOCKERHUB_REPO="junfuchang/superflare"

docker build -t "superflare-base:$VERSION" -f docker/manual/Dockerfile.base .
docker build -t "$DOCKERHUB_REPO:$VERSION-amd64" --build-arg FLARE_BASE_IMAGE="superflare-base:$VERSION" -f docker/manual/Dockerfile.amd64 .
docker build -t "$DOCKERHUB_REPO:$VERSION-arm32v7" --build-arg FLARE_BASE_IMAGE="superflare-base:$VERSION" -f docker/manual/Dockerfile.arm32v7 .
docker build -t "$DOCKERHUB_REPO:$VERSION-arm64v8" --build-arg FLARE_BASE_IMAGE="superflare-base:$VERSION" -f docker/manual/Dockerfile.arm64v8 .

rm RELEASE_VERSION
rm RELEASE_COMMIT

docker images | grep "$DOCKERHUB_REPO"