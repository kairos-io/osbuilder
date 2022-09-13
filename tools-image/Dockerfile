ARG ELEMENTAL_CLI_VERSION=v0.0.15-ae4f000
ARG LEAP_VERSION=15.4
ARG LUET_VERSION=0.32.5
FROM quay.io/costoolkit/elemental-cli:$ELEMENTAL_CLI_VERSION AS elemental
FROM quay.io/luet/base:$LUET_VERSION AS luet
FROM quay.io/costoolkit/releases-teal:grub2-live-0.0.3-2  AS grub2
FROM quay.io/costoolkit/releases-teal:grub2-efi-image-live-0.0.2-2 AS efi

FROM opensuse/leap:$LEAP_VERSION
COPY --from=elemental /usr/bin/elemental /usr/bin/elemental
COPY --from=luet /usr/bin/luet /usr/bin/luet

COPY --from=grub2 / /grub2
COPY --from=efi / /efi

RUN zypper ref && zypper dup -y
RUN zypper ref && zypper in -y xfsprogs parted util-linux-systemd e2fsprogs util-linux udev rsync grub2 dosfstools grub2-x86_64-efi squashfs mtools xorriso lvm2
RUN mkdir /config

COPY ./config.yaml /config/manifest.yaml
COPY ./entrypoint.sh /entrypoint.sh

ENTRYPOINT [ "/entrypoint.sh" ]