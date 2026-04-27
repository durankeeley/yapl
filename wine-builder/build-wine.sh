#!/usr/bin/env bash
set -e

WINE_VERSION="${1:-latest}"
OUTPUT_DIR="/output"

echo "Building Wine ${WINE_VERSION}"

git clone https://gitlab.winehq.org/wine/wine.git

cd wine
if [ "$WINE_VERSION" != "latest" ]; then
    git checkout "wine-${WINE_VERSION}" || echo "Tag wine-${WINE_VERSION} not found, building from HEAD"
fi
COMMIT_SHORT=$(git rev-parse --short HEAD)
cd ..

OUTPUT_NAME="wine-${WINE_VERSION}-${COMMIT_SHORT}"
INSTALL_DIR="/wine-install/${OUTPUT_NAME}"

# Wine 9+ supports WoW64 single-build: one 64-bit binary handles 32-bit apps
# without needing 32-bit Linux libraries. Older versions require the dual build.
use_wow64() {
    [ "$WINE_VERSION" = "latest" ] && return 0
    local major
    major=$(echo "$WINE_VERSION" | cut -d. -f1)
    [ "$major" -ge 9 ]
}

if use_wow64; then
    echo "Building ${OUTPUT_NAME} (WoW64 single-build)"
    mkdir -p wine-build
    cd wine-build
    ../wine/configure --enable-win64 --enable-wow64 --prefix=/usr
    make -j"$(nproc)"
    make install DESTDIR="${INSTALL_DIR}"
    cd ..
else
    echo "Building ${OUTPUT_NAME} (legacy dual 32+64 build)"
    mkdir -p wine64-build wine32-build

    cd wine64-build
    ../wine/configure --enable-win64 --prefix=/usr
    make -j"$(nproc)"
    cd ..

    cd wine32-build
    ../wine/configure --with-wine64=../wine64-build --prefix=/usr
    make -j"$(nproc)"
    cd ..

    make -C wine64-build install DESTDIR="${INSTALL_DIR}"
    make -C wine32-build install DESTDIR="${INSTALL_DIR}"
fi

echo "Creating ${OUTPUT_NAME}.tar.xz"
cd /wine-install
tar -cJf "${OUTPUT_DIR}/${OUTPUT_NAME}.tar.xz" "${OUTPUT_NAME}/"

echo "Done: ${OUTPUT_DIR}/${OUTPUT_NAME}.tar.xz"
