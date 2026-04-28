FROM debian:bookworm

ENV DEBIAN_FRONTEND=noninteractive

# Exclude docs/locales/man pages from all package installs (smaller image).
RUN echo 'path-exclude=/usr/share/doc/*' > /etc/dpkg/dpkg.cfg.d/99-exclude-cruft && \
    echo 'path-exclude=/usr/share/locale/*' >> /etc/dpkg/dpkg.cfg.d/99-exclude-cruft && \
    echo 'path-exclude=/usr/share/man/*' >> /etc/dpkg/dpkg.cfg.d/99-exclude-cruft

RUN dpkg --add-architecture i386 && \
    apt-get update && apt-get install -y --no-install-recommends \
    \
    # Native compilers (ELF host binaries + -m32 for 32-bit satellite build).
    # MinGW cross-compilers (CROSSCC) build all PE DLLs — no clang needed.
    gcc gcc-multilib g++ g++-multilib \
    gcc-mingw-w64-x86-64 gcc-mingw-w64-i686 \
    g++-mingw-w64-x86-64 g++-mingw-w64-i686 \
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
    libv4l-dev \
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

# ccache wraps all compilers (gcc, x86_64-w64-mingw32-gcc, i686-w64-mingw32-gcc, etc.)
# transparently via PATH symlinks in /usr/lib/ccache/.
ENV PATH="/usr/lib/ccache:${PATH}"

WORKDIR /build

VOLUME ["/output"]

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY build-wine.sh /usr/local/bin/build-wine.sh

RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/build-wine.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
