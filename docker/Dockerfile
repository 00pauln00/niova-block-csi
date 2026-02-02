FROM ubuntu:24.04

# Install dependencies
RUN apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y \
    util-linux \
    libblkid1 \
    build-essential \
    autoconf \
    automake \
    libtool \
    pkg-config \
    liburing-dev \
    git \
    wget \
    curl \
    sudo && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /csi /var/niova /etc/niova /opt/niova /usr/local/bin /usr/local/lib /usr/local/include /usr/local/libexec /usr/local/sbin /usr/local/share

ARG BINARY=controller

COPY opt/niova/niova-block-csi-controller /opt/niova/niova-block-csi-controller

COPY opt/niova/niova-block-csi-node /opt/niova/niova-block-csi-node

COPY usr/local/bin /usr/local/bin

COPY usr/local/lib/ /usr/local/lib

COPY usr/local/include/ /usr/local/include

COPY usr/local/libexec/ /usr/local/libexec

COPY usr/local/sbin /usr/local/sbin

COPY usr/local/share /usr/local/share

RUN chmod +x /opt/niova/niova-block-csi-${BINARY}

ENTRYPOINT ["/bin/sh", "-c", "exec /opt/niova/niova-block-csi-${BINARY} \"$@\"", "--"]

