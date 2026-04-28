#!/usr/bin/env bash
set -e

cd /build
/usr/local/bin/build-wine.sh "$@"
