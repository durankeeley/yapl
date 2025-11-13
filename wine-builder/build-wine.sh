#!/usr/bin/env bash
set -e

WINE_VERSION="${1:-latest}"
STAGING="${STAGING:-false}"
OUTPUT_DIR="/output"

echo " Building Wine ${WINE_VERSION} (Staging: ${STAGING})"

# Clone Wine source
if [ "$WINE_VERSION" == "latest" ]; then
  echo "Fetching latest Wine..."
  git clone https://gitlab.winehq.org/wine/wine.git
else
  echo "Fetching Wine version wine-${WINE_VERSION}..."
  git clone https://gitlab.winehq.org/wine/wine.git
  cd wine
  git checkout "wine-${WINE_VERSION}" || {
    echo "Version not found, defaulting to latest master"
  }
  cd ..
fi

# Create build directories
mkdir -p wine64-build wine32-build

# Build 64-bit 
echo "Building 64-bit Wine"
cd wine64-build
../wine/configure --enable-win64
make -j"$(nproc)"
cd ..

# Build 32-bit
echo "Building 32-bit Wine"
cd wine32-build
../wine/configure --with-wine64=../wine64-build
make -j"$(nproc)"
cd ..

# Install both builds to a temporary root
mkdir -p /wine-install
cd wine64-build && make install DESTDIR=/wine-install && cd ..
cd wine32-build && make install DESTDIR=/wine-install && cd ..

# Copy built files to mounted output directory
cp -r /wine-install/* "${OUTPUT_DIR}/"

echo "Wine ${WINE_VERSION} build complete"
