#! /bin/bash
set -e

image="frizinak/inbetween-go-homecam"
name="frizinak-inbetween-go-homecam"
dir="/go/src/github.com/frizinak/inbetween-go-homecam"

docker rm "$name" 2>/dev/null || true
docker build --build-arg DIR="$dir" -t "$image" .
docker create --name "$name" "$image"
docker cp "$name:$dir/dist" ./dist
echo
echo done
