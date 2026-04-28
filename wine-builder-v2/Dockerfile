FROM debian:trixie

ENV DEBIAN_FRONTEND=noninteractive

# Exclude docs/locales/man pages from all package installs (smaller image).
RUN echo 'path-exclude=/usr/share/doc/*' > /etc/dpkg/dpkg.cfg.d/99-exclude-cruft && \
    echo 'path-exclude=/usr/share/locale/*' >> /etc/dpkg/dpkg.cfg.d/99-exclude-cruft && \
    echo 'path-exclude=/usr/share/man/*' >> /etc/dpkg/dpkg.cfg.d/99-exclude-cruft

RUN dpkg --add-architecture i386 && \
    apt-get update && apt-get install -y --no-install-recommends \
    \
    # Native compilers (ELF host binaries).
    # GCC MinGW cross-compilers are kept for Wine <9 legacy dual-build.
    # llvm-mingw (below) handles all PE DLL cross-compilation for Wine 9+.
    gcc gcc-multilib g++ g++-multilib \
    gcc-mingw-w64-x86-64 gcc-mingw-w64-i686 \
    g++-mingw-w64-x86-64 g++-mingw-w64-i686 \
    clang lld llvm \
    \
    # Build tools
    git make flex bison autoconf automake libtool pkg-config \
    ccache wget curl ca-certificates xz-utils build-essential \
    python3 patch \
    \
    # Kernel headers (needed for some Wine subsystems)
    linux-libc-dev linux-libc-dev:i386 \
    \
    # Audio
    libasound2-dev libasound2-dev:i386 \
    libpulse-dev libpulse-dev:i386 \
    \
    # Video / media (ffmpeg for in-game video playback)
    libavcodec-dev libavcodec-dev:i386 \
    libavformat-dev libavformat-dev:i386 \
    libavutil-dev libavutil-dev:i386 \
    libswresample-dev libswresample-dev:i386 \
    libswscale-dev libswscale-dev:i386 \
    libv4l-dev libv4l-dev:i386 \
    \
    # GStreamer (Wine media backend)
    libgstreamer1.0-dev \
    libgstreamer-plugins-base1.0-dev \
    \
    # Graphics / GPU
    libgl1-mesa-dev libgl1-mesa-dev:i386 \
    libosmesa6-dev libosmesa6-dev:i386 \
    libvulkan-dev libvulkan-dev:i386 \
    \
    # Display / input
    libdbus-1-dev libdbus-1-dev:i386 \
    libice-dev libice-dev:i386 \
    libsdl2-dev libsdl2-dev:i386 \
    libwayland-dev libwayland-dev:i386 \
    libx11-dev libx11-dev:i386 \
    libxcomposite-dev libxcomposite-dev:i386 \
    libxcursor-dev libxcursor-dev:i386 \
    libxext-dev libxext-dev:i386 \
    libxi-dev libxi-dev:i386 \
    libxinerama-dev libxinerama-dev:i386 \
    libxkbcommon-dev libxkbcommon-dev:i386 \
    libxkbregistry-dev \
    libxrandr-dev libxrandr-dev:i386 \
    libxrender-dev libxrender-dev:i386 \
    libxxf86vm-dev libxxf86vm-dev:i386 \
    \
    # Fonts / text rendering
    libfontconfig-dev libfontconfig-dev:i386 \
    libfreetype-dev libfreetype-dev:i386 \
    \
    # Network / auth
    libkrb5-dev libkrb5-dev:i386 \
    libpcap-dev libpcap-dev:i386 \
    \
    # Peripherals
    libgphoto2-dev \
    libsane-dev \
    libusb-1.0-0-dev libusb-1.0-0-dev:i386 \
    libudev-dev libudev-dev:i386 \
    \
    # Printing
    libcups2-dev libcups2-dev:i386 \
    \
    # Crypto / TLS
    libgnutls28-dev libgnutls28-dev:i386 \
    \
    # OpenCL
    ocl-icd-opencl-dev ocl-icd-opencl-dev:i386 \
    \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# llvm-mingw 20251007 (LLVM 19) provides Clang-based i686/x86_64-w64-mingw32
# cross-compilers required for --enable-archs=i386,x86_64 WoW64 single-build.
# Wine configure detects -target/-fuse-ld=lld support and enables the mixed
# i386+x86_64 COFF path in dlls/wow64/syscall.c.
# LLVM 20+ (20260421+) introduced i386=1 predefinition and stricter .seh_*
# restrictions that break this; 20251007 matches Wine's own CI image.
# Note: Wine CI uses this version for arm64; on amd64 we opt into it for the
# superior --enable-archs single-process WoW64 (what Valve/Proton uses).
ARG LLVM_MINGW_VERSION=20251007
RUN wget -q "https://github.com/mstorsjo/llvm-mingw/releases/download/${LLVM_MINGW_VERSION}/llvm-mingw-${LLVM_MINGW_VERSION}-ucrt-ubuntu-22.04-x86_64.tar.xz" \
    && tar -xf "llvm-mingw-${LLVM_MINGW_VERSION}-ucrt-ubuntu-22.04-x86_64.tar.xz" -C /opt \
    && rm "llvm-mingw-${LLVM_MINGW_VERSION}-ucrt-ubuntu-22.04-x86_64.tar.xz"

# llvm-mingw before /usr/lib/ccache: ensures i686-w64-mingw32-gcc resolves to
# llvm-mingw's clang symlink rather than gcc-mingw-w64's GCC.
# /usr/lib/ccache wraps native compilers (gcc, clang) transparently.
ENV PATH="/opt/llvm-mingw-${LLVM_MINGW_VERSION}-ucrt-ubuntu-22.04-x86_64/bin:/usr/lib/ccache:${PATH}"

WORKDIR /build

VOLUME ["/output"]

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY build-wine.sh /usr/local/bin/build-wine.sh

RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/build-wine.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
