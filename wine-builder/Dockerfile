FROM debian:bullseye

ENV DEBIAN_FRONTEND=noninteractive

RUN dpkg --add-architecture i386 && \
    apt-get update && apt-get install -y --no-install-recommends \
    gcc gcc-multilib gcc-mingw-w64-x86-64 gcc-mingw-w64-i686 \
    git make flex bison autoconf automake libtool pkg-config \
    ccache wget curl ca-certificates xz-utils \
    build-essential \
    libasound2-dev libasound2-dev:i386 \
    libcups2-dev libcups2-dev:i386 \
    libdbus-1-dev libdbus-1-dev:i386 \
    libfontconfig-dev libfontconfig-dev:i386 \
    libfreetype-dev libfreetype-dev:i386 \
    libgl1-mesa-dev libgl1-mesa-dev:i386 \
    libgnutls28-dev libgnutls28-dev:i386 \
    libgphoto2-dev libgphoto2-dev:i386 \
    libice-dev libice-dev:i386 \
    libkrb5-dev libkrb5-dev:i386 \
    libosmesa6-dev libosmesa6-dev:i386 \
    libpcap-dev libpcap-dev:i386 \
    libpulse-dev libpulse-dev:i386 \
    libsane-dev libsane-dev:i386 \
    libsdl2-dev libsdl2-dev:i386 \
    libudev-dev libudev-dev:i386 \
    libusb-1.0-0-dev libusb-1.0-0-dev:i386 \
    libvulkan-dev libvulkan-dev:i386 \
    libwayland-dev libwayland-dev:i386 \
    libx11-dev libx11-dev:i386 \
    libxcomposite-dev libxcomposite-dev:i386 \
    libxcursor-dev libxcursor-dev:i386 \
    libxext-dev libxext-dev:i386 \
    libxi-dev libxi-dev:i386 \
    libxinerama-dev libxinerama-dev:i386 \
    libxrandr-dev libxrandr-dev:i386 \
    libxrender-dev libxrender-dev:i386 \
    libxxf86vm-dev libxxf86vm-dev:i386 && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

ENV CC="ccache gcc"
ENV CXX="ccache g++"

WORKDIR /build

VOLUME ["/output"]

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY build-wine.sh /usr/local/bin/build-wine.sh

RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/build-wine.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
