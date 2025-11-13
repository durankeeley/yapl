# Base OS
FROM debian:bullseye

ENV DEBIAN_FRONTEND=noninteractive

# Version argument
ARG WINE_VERSION=latest
ENV WINE_VERSION=${WINE_VERSION}

RUN dpkg --add-architecture i386 && \
    apt-get update && apt-get install -y --no-install-recommends \
    gcc gcc-multilib gcc-mingw-w64-x86-64 gcc-mingw-w64-i686 \
    git make flex bison autoconf automake libtool pkg-config \
    ccache wget curl sudo vim nano \
    build-essential ca-certificates xz-utils \
    libasound2-dev:amd64 libasound2-dev:i386 libcapi20-dev:amd64 libcapi20-dev:i386 \
    libcups2-dev:amd64 libcups2-dev:i386 libdbus-1-dev:amd64 libdbus-1-dev:i386 \
    libfontconfig-dev:amd64 libfontconfig-dev:i386 libfreetype-dev:amd64 libfreetype-dev:i386 \
    libgl1-mesa-dev:amd64 libgl1-mesa-dev:i386 libgnutls28-dev:amd64 libgnutls28-dev:i386 \
    libgphoto2-dev:amd64 libgphoto2-dev:i386 libice-dev:amd64 libice-dev:i386 \
    libkrb5-dev:amd64 libkrb5-dev:i386 libosmesa6-dev:amd64 libosmesa6-dev:i386 \
    libpcap-dev:amd64 libpcap-dev:i386 libpulse-dev:amd64 libpulse-dev:i386 \
    libsane-dev:amd64 libsane-dev:i386 libsdl2-dev:amd64 libsdl2-dev:i386 \
    libudev-dev:amd64 libudev-dev:i386 libusb-1.0-0-dev:amd64 libusb-1.0-0-dev:i386 \
    libvulkan-dev:amd64 libvulkan-dev:i386 libwayland-dev:amd64 libwayland-dev:i386 \
    libx11-dev:amd64 libx11-dev:i386 libxcomposite-dev:amd64 libxcomposite-dev:i386 \
    libxcursor-dev:amd64 libxcursor-dev:i386 libxext-dev:amd64 libxext-dev:i386 \
    libxi-dev:amd64 libxi-dev:i386 libxinerama-dev:amd64 libxinerama-dev:i386 \
    libxrandr-dev:amd64 libxrandr-dev:i386 libxrender-dev:amd64 libxrender-dev:i386 \
    libxxf86vm-dev:amd64 libxxf86vm-dev:i386 && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Speed up rebuilds
ENV CC="ccache gcc"
ENV CXX="ccache g++"

WORKDIR /build

VOLUME ["/output"]

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY build-wine.sh /usr/local/bin/build-wine.sh

RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/build-wine.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
