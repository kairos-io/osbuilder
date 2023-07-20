# https://quay.io/repository/kairos/packages?tab=tags&tag=latest
ARG LEAP_VERSION=15.5
ARG LUET_VERSION=0.33.0
FROM quay.io/luet/base:$LUET_VERSION AS luet

FROM golang:1.20 as enki
ENV CGO_ENABLED=0
COPY ./enki /src/enki
WORKDIR /src/enki
RUN go mod download
# Set arg/env after go mod download, otherwise we invalidate the cached layers due to the commit changing easily
ARG ENKI_VERSION=0.0.1
ARG ENKI_COMMIT=""
ENV ENKI_VERSION=${ENKI_VERSION}
ENV ENKI_COMMIT=${ENKI_COMMIT}
RUN go build \
    -ldflags "-w -s \
    -X github.com/kairos-io/enki/internal/version.version=$ENKI_VERSION \
    -X github.com/kairos-io/enki/internal/version.gitCommit=$ENKI_COMMIT" \
    -o /usr/bin/enki

FROM opensuse/leap:$LEAP_VERSION as default
COPY --from=luet /usr/bin/luet /usr/bin/luet
ENV LUET_NOLOCK=true
ENV TMPDIR=/tmp
COPY luet.yaml /etc/luet/luet.yaml

RUN zypper ref && zypper dup -y
## ISO+ Arm image + Netboot + cloud images Build depedencies
RUN zypper ref && zypper in -y bc qemu-tools jq cdrtools docker git curl gptfdisk kpartx sudo xfsprogs parted util-linux-systemd e2fsprogs curl util-linux udev rsync grub2 dosfstools grub2-x86_64-efi squashfs mtools xorriso lvm2 zstd

## Live CD artifacts
RUN luet install -y livecd/grub2 --system-target /grub2
RUN luet install -y livecd/grub2-efi-image --system-target /efi

## RPI64
RUN luet install -y firmware/u-boot-rpi64 firmware/raspberrypi-firmware firmware/raspberrypi-firmware-config firmware/raspberrypi-firmware-dt --system-target /rpi/

## PineBook64 Pro
RUN luet install -y arm-vendor-blob/u-boot-rockchip --system-target /pinebookpro/u-boot

 ## RAW images
RUN luet install -y static/grub-efi --system-target /raw/grub
RUN luet install -y static/grub-config --system-target /raw/grubconfig
RUN luet install -y static/grub-artifacts --system-target /raw/grubartifacts

# remove luet tmp files. Side effect of setting the system-target is that it treats it as a root fs
# so temporal files are stored in each dir
RUN rm -Rf /grub2/var/tmp
RUN rm -Rf /efi/var/tmp
RUN rm -Rf /rpi/var/tmp
RUN rm -Rf /pinebookpro/u-boot/var/tmp
RUN rm -Rf /raw/grub/var/tmp
RUN rm -Rf /raw/grubconfig/var/tmp
RUN rm -Rf /raw/grubartifacts/var/tmp

RUN mkdir /config

# ISO build config
COPY ./config.yaml /config/manifest.yaml
COPY ./entrypoint.sh /entrypoint.sh
COPY ./add-cloud-init.sh /add-cloud-init.sh
COPY ./os-release.tmpl /os-release.tmpl
COPY ./update-os-release.sh /update-os-release.sh

# ARM helpers
COPY ./build-arm-image.sh /build-arm-image.sh
COPY ./arm /arm
COPY ./prepare_arm_images.sh /prepare_arm_images.sh

# RAW images helpers
COPY ./gce.sh /gce.sh
COPY ./raw-images.sh /raw-images.sh
COPY ./azure.sh /azure.sh
COPY ./netboot.sh /netboot.sh

COPY defaults.yaml /defaults.yaml

COPY --from=enki /usr/bin/enki /usr/bin/enki

ENTRYPOINT [ "/entrypoint.sh" ]
