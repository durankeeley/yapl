#!/usr/bin/env bash
set -e

# ── Input parsing ──────────────────────────────────────────────────────
# Accepts: VERSION[-staging][-tkg][-wow64] or VERSION-x86
# Examples: "11.7", "11.7-staging", "11.7-staging-wow64",
#           "11.7-staging-tkg-wow64", "11.7-x86"
# tkg always implies staging.
WINE_VERSION="${WINE_VERSION:-${1:-latest}}"
OUTPUT_DIR="/output"

BUILD_WOW64=0; IS_STAGING=0; IS_TKG=0; IS_X86=0

# Strip flags right-to-left so combinations parse correctly.
[[ "$WINE_VERSION" == *-wow64   ]] && { BUILD_WOW64=1; WINE_VERSION="${WINE_VERSION%-wow64}";   }
[[ "$WINE_VERSION" == *-tkg     ]] && { IS_TKG=1;      WINE_VERSION="${WINE_VERSION%-tkg}";     }
[[ "$WINE_VERSION" == *-staging ]] && { IS_STAGING=1;  WINE_VERSION="${WINE_VERSION%-staging}"; }
[[ "$WINE_VERSION" == *-x86     ]] && { IS_X86=1;      WINE_VERSION="${WINE_VERSION%-x86}";     }
[ "$IS_TKG" -eq 1 ] && IS_STAGING=1

VARIANT=""
if [ "$IS_X86" -eq 1 ]; then
    VARIANT="-x86"
else
    [ "$IS_STAGING" -eq 1 ] && VARIANT="${VARIANT}-staging"
    [ "$IS_TKG"     -eq 1 ] && VARIANT="${VARIANT}-tkg"
fi

WOW64_LABEL=""; [ "$BUILD_WOW64" -eq 1 ] && WOW64_LABEL="-wow64"
echo "Building Wine ${WINE_VERSION}${VARIANT}${WOW64_LABEL}"

# ── Clone wine ─────────────────────────────────────────────────────────
if [ "$WINE_VERSION" = "latest" ]; then
    git clone --depth=1 https://gitlab.winehq.org/wine/wine.git
else
    git clone --depth=1 --branch "wine-${WINE_VERSION}" \
        https://gitlab.winehq.org/wine/wine.git
fi
COMMIT_SHORT=$(git -C wine rev-parse --short HEAD)
WINE_MAJOR=$([ "$WINE_VERSION" = "latest" ] && echo 999 \
    || echo "$WINE_VERSION" | cut -d. -f1)

OUTPUT_NAME="wine-${WINE_VERSION}-${COMMIT_SHORT}${VARIANT}"
INSTALL_DIR="/wine-install/${OUTPUT_NAME}"

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

# ── Build flags ────────────────────────────────────────────────────────
CONFIGURE_BASE="--prefix=/usr --disable-tests --without-oss --disable-winemenubuilder"

# Arch-targeted flags matching Kron4ek's approach (no -g → 3-4x smaller binaries).
# CROSS variants omit -mfpmath=sse: Wine's PE makefile adds -mlong-double-64 for i386
# Windows targets, and combining it with -mfpmath=sse triggers a GCC ICE in assign_stack_local_1
# (seen in oleaut32/vartype.c with gcc-mingw-w64 on Bookworm).
CFLAGS_X64="-march=x86-64 -msse3 -mfpmath=sse -O2 -ftree-vectorize"
CFLAGS_X32="-march=i686  -msse2 -mfpmath=sse -O2 -ftree-vectorize"
CROSSCFLAGS_X64="-march=x86-64 -msse3 -O2 -ftree-vectorize"
# i386-windows: Wine's PE makefile appends -mlong-double-64 -mpreferred-stack-boundary=2;
# any -msse* or -mfpmath flag on top triggers a GCC ICE in assign_stack_local_1.
CROSSCFLAGS_X32="-march=i686 -O2"
LDFLAGS_COMMON="-Wl,-O1,--sort-common,--as-needed"

# GCC 12+ (Bookworm) promotes several old C patterns to errors in pre-9 Wine.
LEGACY_COMPAT="-Wno-error=implicit-function-declaration \
    -Wno-error=incompatible-pointer-types \
    -Wno-error=int-conversion \
    -Wno-error=discarded-qualifiers \
    -Wno-error=deprecated-declarations"

EXTRA=""; [ "$WINE_MAJOR" -lt 9 ] && EXTRA="${LEGACY_COMPAT}"

# ── Standalone x86 build ───────────────────────────────────────────────
if [ "$IS_X86" -eq 1 ]; then
    echo "Building ${OUTPUT_NAME} (standalone 32-bit)"
    mkdir -p wine32-only
    (
        export CC="ccache gcc -m32"
        export CXX="ccache g++ -m32"
        export CFLAGS="${CFLAGS_X32} ${EXTRA}"
        export CXXFLAGS="${CFLAGS}"
        export CROSSCC="i686-w64-mingw32-gcc"
        export CROSSCXX="i686-w64-mingw32-g++"
        export CROSSCFLAGS="${CROSSCFLAGS_X32}"
        export LDFLAGS="${LDFLAGS_COMMON}"
        export CROSSLDFLAGS="${LDFLAGS_COMMON}"
        export PKG_CONFIG_LIBDIR="/usr/lib/i386-linux-gnu/pkgconfig"
        cd wine32-only
        ../wine/configure ${CONFIGURE_BASE}
        make -j"${CORES}"
    )
    mkdir -p "${INSTALL_DIR}"
    make -C wine32-only install DESTDIR="${INSTALL_DIR}"
    echo "Creating ${OUTPUT_NAME}.tar.xz"
    cd /wine-install
    XZ_OPT="-9 -T 0" tar -cJf "${OUTPUT_DIR}/${OUTPUT_NAME}.tar.xz" "${OUTPUT_NAME}/"
    echo "Done: ${OUTPUT_DIR}/${OUTPUT_NAME}.tar.xz"
    exit 0
fi

# ── Dual 64+32 build ──────────────────────────────────────────────────
echo "Building ${OUTPUT_NAME} (dual 64+32, Kron4ek style)"
mkdir -p wine64-build wine32-build

# 64-bit build: CROSSCC targets x86_64-windows PE DLLs.
(
    export CROSSCC="x86_64-w64-mingw32-gcc"
    export CROSSCXX="x86_64-w64-mingw32-g++"
    export CFLAGS="${CFLAGS_X64} ${EXTRA}"
    export CXXFLAGS="${CFLAGS}"
    export CROSSCFLAGS="${CROSSCFLAGS_X64}"
    export CROSSCXXFLAGS="${CROSSCFLAGS_X64}"
    export LDFLAGS="${LDFLAGS_COMMON}"
    export CROSSLDFLAGS="${LDFLAGS_COMMON}"
    cd wine64-build
    ../wine/configure --enable-win64 ${CONFIGURE_BASE}
    make -j"${CORES}"
)

# 32-bit satellite: links against the 64-bit tree.
# --with-wine-tools uses wine64's tools (widl, winebuild, wrc) directly
# since they run natively on x86_64 — no separate 32-bit tools build needed.
(
    export CC="ccache gcc -m32"
    export CXX="ccache g++ -m32"
    export CROSSCC="i686-w64-mingw32-gcc"
    export CROSSCXX="i686-w64-mingw32-g++"
    export CFLAGS="${CFLAGS_X32} ${EXTRA}"
    export CXXFLAGS="${CFLAGS}"
    export CROSSCFLAGS="${CROSSCFLAGS_X32}"
    export CROSSCXXFLAGS="${CROSSCFLAGS_X32}"
    export LDFLAGS="${LDFLAGS_COMMON}"
    export CROSSLDFLAGS="${LDFLAGS_COMMON}"
    export PKG_CONFIG_LIBDIR="/usr/lib/i386-linux-gnu/pkgconfig"
    cd wine32-build
    ../wine/configure \
        --with-wine64=../wine64-build \
        --with-wine-tools=../wine64-build \
        ${CONFIGURE_BASE}
    make -j"${CORES}"
)

mkdir -p "${INSTALL_DIR}"
make -C wine64-build install DESTDIR="${INSTALL_DIR}"
make -C wine32-build install DESTDIR="${INSTALL_DIR}"

# ── WoW64 post-processing (Wine 9+ only) ──────────────────────────────
# Mirrors Kron4ek's amd64-wow64: removes i386-unix ELF loader stubs so all
# 32-bit Windows code runs inside the 64-bit wine process via PE DLL thunking.
if [ "$BUILD_WOW64" -eq 1 ] && [ "$WINE_MAJOR" -ge 9 ]; then
    WOW64_NAME="${OUTPUT_NAME}-wow64"
    WOW64_DIR="/wine-install/${WOW64_NAME}"
    cp -r "${INSTALL_DIR}" "${WOW64_DIR}"
    rm -rf "${WOW64_DIR}/usr/lib/wine/i386-unix"
    rm -f "${WOW64_DIR}/usr/bin/wine" "${WOW64_DIR}/usr/bin/wine-preloader"
    [ -f "${WOW64_DIR}/usr/bin/wine64" ] && \
        cp "${WOW64_DIR}/usr/bin/wine64" "${WOW64_DIR}/usr/bin/wine"
    echo "Creating ${WOW64_NAME}.tar.xz"
    cd /wine-install
    XZ_OPT="-9 -T 0" tar -cJf "${OUTPUT_DIR}/${WOW64_NAME}.tar.xz" "${WOW64_NAME}/"
    echo "Done: ${OUTPUT_DIR}/${WOW64_NAME}.tar.xz"
else
    echo "Creating ${OUTPUT_NAME}.tar.xz"
    cd /wine-install
    XZ_OPT="-9 -T 0" tar -cJf "${OUTPUT_DIR}/${OUTPUT_NAME}.tar.xz" "${OUTPUT_NAME}/"
    echo "Done: ${OUTPUT_DIR}/${OUTPUT_NAME}.tar.xz"
fi
