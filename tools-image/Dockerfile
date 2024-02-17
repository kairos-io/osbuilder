# https://quay.io/repository/kairos/packages?tab=tags&tag=latest
ARG FEDORA_VERSION=40
ARG LUET_VERSION=0.35.0
ARG ENKI_VERSION=v0.0.17-rc2

FROM quay.io/luet/base:$LUET_VERSION AS luet
FROM quay.io/kairos/enki:${ENKI_VERSION} as enki

FROM fedora:$FEDORA_VERSION as default
RUN dnf -y update
## ISO+ Arm image + Netboot + cloud images Build depedencies
RUN dnf in -y bc qemu-tools jq genisoimage docker git curl gdisk kpartx sudo xfsprogs parted e2fsprogs curl  \
    util-linux udev rsync grub2 dosfstools mtools xorriso lvm2 zstd sbsigntools squashfs-tools openssl \
    python3-cryptography python3-pefile # ukify deps
# systemd-ukify systemd-boot
# Install grub2-efi-x64 only on x86 arches
RUN if [ "$(uname -m)" == "x86_64" ]; then dnf install -y grub2-efi-x64; fi
# Install grub2-efi-arm64 only on arm64 arches
RUN if [ "$(uname -m)" == "aarch64" ]; then dnf install -y grub2-efi-aa64; fi

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
## We can install both arches, as the artifacts are named differently
RUN luet install --config /tmp/luet-amd64.yaml -y system/systemd-boot
RUN luet install --config /tmp/luet-arm64.yaml -y system/systemd-boot
# ukify + measure
RUN luet install -y system/systemd-ukify

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
COPY ./os-release.tmpl /os-release.tmpl
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
