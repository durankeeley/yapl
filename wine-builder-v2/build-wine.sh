#!/usr/bin/env bash
set -e

# ── Input parsing ──────────────────────────────────────────────────────
# Accepts: VERSION[-staging][-tkg]
# v2 is always WoW64 for Wine 9+ (--enable-archs single-process).
# No -wow64 or -x86 flags — use v1 for those variants.
# tkg always implies staging.
WINE_VERSION="${WINE_VERSION:-${1:-latest}}"
OUTPUT_DIR="/output"

IS_STAGING=0; IS_TKG=0

[[ "$WINE_VERSION" == *-tkg     ]] && { IS_TKG=1;      WINE_VERSION="${WINE_VERSION%-tkg}";     }
[[ "$WINE_VERSION" == *-staging ]] && { IS_STAGING=1;  WINE_VERSION="${WINE_VERSION%-staging}"; }
[ "$IS_TKG" -eq 1 ] && IS_STAGING=1

VARIANT=""
[ "$IS_STAGING" -eq 1 ] && VARIANT="${VARIANT}-staging"
[ "$IS_TKG"     -eq 1 ] && VARIANT="${VARIANT}-tkg"

echo "Building Wine ${WINE_VERSION}${VARIANT}"

# ── Clone wine ─────────────────────────────────────────────────────────
# Shallow clone: release tags have pre-generated configure/Makefile.in so
# autoreconf is not needed (unlike MR CI builds).
if [ "$WINE_VERSION" = "latest" ]; then
    git clone --depth=1 https://gitlab.winehq.org/wine/wine.git
else
    git clone --depth=1 --branch "wine-${WINE_VERSION}" https://gitlab.winehq.org/wine/wine.git
fi

COMMIT_SHORT=$(git -C wine rev-parse --short HEAD)
WINE_MAJOR=$([ "$WINE_VERSION" = "latest" ] && echo 999 \
    || echo "$WINE_VERSION" | cut -d. -f1)

CORES=$(nproc)
[ "$CORES" -gt 4 ] && CORES=4

# ── Helper: apply a patch file best-effort ────────────────────────────
apply_patch() {
    local abs_patch name
    [ -f "$1" ] || return 0
    abs_patch="$(realpath "$1")"
    name="$(basename "$1")"
    if (cd wine && git apply --check --whitespace=fix "${abs_patch}" 2>/dev/null); then
        (cd wine && git apply --whitespace=fix "${abs_patch}")
        echo "  Applied: ${name}"
    else
        echo "  Skipped: ${name}"
    fi
}

# ── Apply wine-staging patches ─────────────────────────────────────────
if [ "$IS_STAGING" -eq 1 ]; then
    echo "Fetching wine-staging v${WINE_VERSION}"
    if [ "$WINE_VERSION" = "latest" ]; then
        git clone --depth=1 https://gitlab.winehq.org/wine/wine-staging.git
    else
        git clone --depth=1 --branch "v${WINE_VERSION}" \
            https://gitlab.winehq.org/wine/wine-staging.git
    fi
    # wine-staging replaced patches/patchinstall.sh with staging/patchinstall.py.
    # Support both to handle old and new repo layouts.
    PATCHINSTALL_PY=$(find wine-staging -name "patchinstall.py" 2>/dev/null | head -1)
    PATCHINSTALL_SH=$(find wine-staging -name "patchinstall.sh" 2>/dev/null | head -1)
    if [ -n "$PATCHINSTALL_PY" ]; then
        python3 "$(realpath "${PATCHINSTALL_PY}")" --destdir="$(realpath wine)" --all
    elif [ -n "$PATCHINSTALL_SH" ]; then
        (cd wine && "$(realpath "${PATCHINSTALL_SH}")" DESTDIR="$(pwd)" --all)
    else
        echo "ERROR: patchinstall not found in wine-staging clone" >&2
        exit 1
    fi
fi

# ── Apply tkg patches from wine-tkg-git ───────────────────────────────
# Source: https://github.com/Frogging-Family/wine-tkg-git/tree/master/wine-tkg-git
# Patches applied:
#   - proton-win10-default  : ensures Win10 compatibility in old prefixes
#   - proton_battleye       : BattlEye anti-cheat bridge (needs Proton BE runtime)
#   - proton-eac_bridge     : Easy Anti-Cheat bridge (needs Proton EAC runtime)
#   - wow64_loader_hack     : EAC expects binary named wine64 in WoW64 builds
#   - misc/fastsync         : ntsync patches for Wine 9 (built-in from Wine 10.15+)
if [ "$IS_TKG" -eq 1 ]; then
    echo "Fetching wine-tkg patches"
    git clone --depth=1 https://github.com/Frogging-Family/wine-tkg-git.git
    PATCHES_DIR="wine-tkg-git/wine-tkg-git/wine-tkg-patches"

    apply_patch "${PATCHES_DIR}/proton/proton-win10-default/proton-win10-default.patch"
    apply_patch "${PATCHES_DIR}/proton-tkg-specific/proton_battleye/proton_battleye.patch"
    apply_patch "${PATCHES_DIR}/proton-tkg-specific/proton_eac/proton-eac_bridge.patch"
    apply_patch "${PATCHES_DIR}/proton-tkg-specific/proton_eac/wow64_loader_hack.patch"

    # ntsync merged into Wine 10.15; only apply for Wine 9 and below.
    if [ "$WINE_MAJOR" -le 9 ]; then
        for patch in "${PATCHES_DIR}/misc/fastsync/"*staging*.patch; do
            apply_patch "${patch}"
        done
    fi

    rm -rf wine-tkg-git
fi

# ── ntdll-ForceBottomUpAlloc (wine-staging) ───────────────────────────
# wine-tkg-git disables this patchset for Star Citizen (earlyhotfixer),
# but it is required for 32-bit games (e.g. NFS MW 2005) where Linux's
# top-down mmap causes pool allocations to land at the top of committed
# memory, placing the last pool object's fields past the committed boundary.
# wine-staging's patchinstall.py --all applies these patches; however,
# tkg patches may revert them. Re-applying them here is safe: apply_patch
# runs git apply --check first and skips any patch already present.
if [ "$IS_STAGING" -eq 1 ] && [ -d "wine-staging" ]; then
    FBTBA_DIR="wine-staging/patches/ntdll-ForceBottomUpAlloc"
    if [ -d "$FBTBA_DIR" ]; then
        echo "Re-applying ntdll-ForceBottomUpAlloc patches"
        for patch in "${FBTBA_DIR}"/*.patch; do
            apply_patch "${patch}"
        done
    fi
fi

# ── Host CFLAGS ───────────────────────────────────────────────────────
# Suppress Wine's default -g (debug info) to keep binaries ~6x smaller.
# These apply to the Unix ELF host objects (ntdll.so, kernel32.so, etc.).
# The i386/x86_64 PE cross-compilers are set via make vars below.
export CFLAGS="-O2 -ftree-vectorize"
export CXXFLAGS="${CFLAGS}"

# ── Wine 9+: true WoW64 single-build (Valve/Proton approach) ──────────
# --enable-archs=i386,x86_64: single-process WoW64, no i386-unix ELF stubs.
# llvm-mingw 20251007 (LLVM 19) provides i686-w64-mingw32-gcc as a clang
# symlink. Wine configure detects -target/-fuse-ld=lld and enables the mixed
# i386+x86_64 COFF path (dlls/wow64/syscall.c).
# ccache wraps llvm-mingw compilers via explicit make vars (not PATH symlinks,
# since /usr/lib/ccache only covers system-installed compilers).
if [ "$WINE_MAJOR" -ge 9 ]; then
    OUTPUT_NAME="wine-${WINE_VERSION}-${COMMIT_SHORT}${VARIANT}-wow64"
    INSTALL_DIR="/wine-install/${OUTPUT_NAME}"
    echo "Building ${OUTPUT_NAME} (Wine 9+ --enable-archs WoW64 single-build)"

    mkdir -p wine-build && cd wine-build

    ../wine/configure \
        --enable-archs=i386,x86_64 \
        --prefix=/usr \
        --disable-tests --without-oss --disable-winemenubuilder

    make -j"${CORES}" \
        "i386_CC=ccache i686-w64-mingw32-gcc" \
        "i386_CXX=ccache i686-w64-mingw32-g++" \
        "x86_64_CC=ccache x86_64-w64-mingw32-gcc" \
        "x86_64_CXX=ccache x86_64-w64-mingw32-g++"

    mkdir -p "${INSTALL_DIR}"
    make install DESTDIR="${INSTALL_DIR}"
    cd ..

    # wine-tkg's wow64_loader_hack.patch makes ntdll look for wine64 as the WoW64
    # loader, but --enable-archs single-process builds only install 'wine' (no wine64).
    # Create wine64 symlinks so the tkg patch resolves correctly and external tools
    # (EAC, YAPL) that look for wine64 find it.
    ln -sf wine "${INSTALL_DIR}/usr/bin/wine64"
    ln -sf wine "${INSTALL_DIR}/usr/lib/wine/x86_64-unix/wine64"
else
    # ── Wine <9: classic dual 64+32 GCC build ─────────────────────────
    # --enable-archs does not exist; fall back to --with-wine64.
    OUTPUT_NAME="wine-${WINE_VERSION}-${COMMIT_SHORT}${VARIANT}"
    INSTALL_DIR="/wine-install/${OUTPUT_NAME}"
    echo "Building ${OUTPUT_NAME} (Wine <9 dual 32+64 GCC build)"

    # No -g: omitting debug symbols is the single biggest size reduction (~3-4x smaller).
    # GCC 12+ promotes several old C patterns to hard errors that pre-9 Wine triggers.
    export CFLAGS="-O2 \
        -Wno-error=implicit-function-declaration \
        -Wno-error=incompatible-pointer-types \
        -Wno-error=int-conversion \
        -Wno-error=discarded-qualifiers \
        -Wno-error=deprecated-declarations"

    mkdir -p wine64-build wine32-build

    cd wine64-build
    ../wine/configure --enable-win64 --prefix=/usr \
        --disable-tests --without-oss --disable-winemenubuilder
    make -j"${CORES}"
    cd ..

    cd wine32-build
    ../wine/configure --with-wine64=../wine64-build --prefix=/usr \
        --disable-tests --without-oss --disable-winemenubuilder
    make -j"${CORES}"
    cd ..

    mkdir -p "${INSTALL_DIR}"
    make -C wine64-build install DESTDIR="${INSTALL_DIR}"
    make -C wine32-build install DESTDIR="${INSTALL_DIR}"
fi

echo "Creating ${OUTPUT_NAME}.tar.xz"
cd /wine-install
XZ_OPT="-9 -T 0" tar -cJf "${OUTPUT_DIR}/${OUTPUT_NAME}.tar.xz" "${OUTPUT_NAME}/"
echo "Done: ${OUTPUT_DIR}/${OUTPUT_NAME}.tar.xz"
