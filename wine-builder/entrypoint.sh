#!/usr/bin/env bash
set -e

if [ -z "$1" ]; then
  echo "Usage: docker run wine-builder <wine-version>"
  exit 1
fi

cd /build
/usr/local/bin/build-wine.sh "$@"
