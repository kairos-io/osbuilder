# https://quay.io/repository/kairos/packages?tab=tags&tag=latest
ARG LEAP_VERSION=15.6
ARG LUET_VERSION=0.35.5
ARG ENKI_VERSION=0.2.2

FROM quay.io/luet/base:$LUET_VERSION AS luet
FROM quay.io/kairos/enki:v${ENKI_VERSION} as enki

FROM opensuse/leap:$LEAP_VERSION as default
RUN zypper ref && zypper dup -y
## ISO+ Arm image + Netboot + cloud images Build depedencies
RUN zypper ref && zypper in -y bc qemu-tools jq cdrtools docker git curl gptfdisk kpartx sudo xfsprogs parted binutils \
    util-linux-systemd e2fsprogs curl util-linux udev rsync grub2 dosfstools grub2-x86_64-efi squashfs mtools xorriso lvm2 zstd
COPY --from=luet /usr/bin/luet /usr/bin/luet
ENV LUET_NOLOCK=true
ENV TMPDIR=/tmp
ARG TARGETARCH
# copy both arches
COPY luet-arm64.yaml /tmp/luet-arm64.yaml
COPY luet-amd64.yaml /tmp/luet-amd64.yaml
# Set the default luet config to the current build arch
RUN mkdir -p /etc/luet/
RUN cp /tmp/luet-${TARGETARCH}.yaml /etc/luet/luet.yaml

## Uki artifacts, will be set under the /usr/kairos directory
RUN luet install -y system/systemd-boot

## Live CD artifacts
RUN luet install -y livecd/grub2 --system-target /grub2
RUN luet install -y livecd/grub2-efi-image --system-target /efi

## RPI64
RUN luet install -y firmware/u-boot-rpi64 firmware/raspberrypi-firmware firmware/raspberrypi-firmware-config firmware/raspberrypi-firmware-dt --system-target /rpi/

## PineBook64 Pro
RUN luet install -y arm-vendor-blob/u-boot-rockchip --system-target /pinebookpro/u-boot

## Odroid fw
RUN luet install -y firmware/odroid-c2 --system-target /firmware/odroid-c2

## RAW images for current arch
RUN luet install -y static/grub-efi --system-target /raw/grub
RUN luet install -y static/grub-config --system-target /raw/grubconfig
RUN luet install -y static/grub-artifacts --system-target /raw/grubartifacts

## RAW images for arm64
# Luet will install this artifacts from the current arch repo, so in x86 it will
# get them from the x86 repo and we want it to do it from the arm64 repo, even on x86
# so we use the arm64 luet config and use that to install those on x86
# This is being used by the prepare_arm_images.sh and build-arch-image.sh scripts
RUN luet install --config /tmp/luet-arm64.yaml -y static/grub-efi --system-target /arm/raw/grubefi
RUN luet install --config /tmp/luet-arm64.yaml -y static/grub-config --system-target /arm/raw/grubconfig
RUN luet install --config /tmp/luet-arm64.yaml -y static/grub-artifacts --system-target /arm/raw/grubartifacts

# kairos-agent so we can use the pull-image
RUN luet install -y system/kairos-agent

# remove luet tmp files. Side effect of setting the system-target is that it treats it as a root fs
# so temporal files are stored in each dir
RUN rm -Rf /grub2/var/tmp
RUN rm -Rf /grub2/var/cache
RUN rm -Rf /efi/var/tmp
RUN rm -Rf /efi/var/cache
RUN rm -Rf /rpi/var/tmp
RUN rm -Rf /rpi/var/cache
RUN rm -Rf /pinebookpro/u-boot/var/tmp
RUN rm -Rf /pinebookpro/u-boot/var/cache
RUN rm -Rf /firmware/odroid-c2/var/tmp
RUN rm -Rf /firmware/odroid-c2/var/cache
RUN rm -Rf /raw/grub/var/tmp
RUN rm -Rf /raw/grub/var/cache
RUN rm -Rf /raw/grubconfig/var/tmp
RUN rm -Rf /raw/grubconfig/var/cache
RUN rm -Rf /raw/grubartifacts/var/tmp
RUN rm -Rf /raw/grubartifacts/var/cache
RUN rm -Rf /arm/raw/grubefi/var/tmp
RUN rm -Rf /arm/raw/grubefi/var/cache
RUN rm -Rf /arm/raw/grubconfig/var/tmp
RUN rm -Rf /arm/raw/grubconfig/var/cache
RUN rm -Rf /arm/raw/grubartifacts/var/tmp
RUN rm -Rf /arm/raw/grubartifacts/var/cache

RUN mkdir /config

# ISO build config
COPY ./config.yaml /config/manifest.yaml
COPY ./entrypoint.sh /entrypoint.sh
COPY ./add-cloud-init.sh /add-cloud-init.sh
COPY ./kairos-release.tmpl /kairos-release.tmpl
COPY ./ipxe.tmpl /ipxe.tmpl
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

COPY --from=enki /enki /usr/bin/enki

ENTRYPOINT [ "/entrypoint.sh" ]
