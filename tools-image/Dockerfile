ARG ELEMENTAL_CLI_VERSION=0.20220921
ARG LEAP_VERSION=15.4
ARG LUET_VERSION=0.32.5
FROM quay.io/kairos/packages:elemental-cli-system-0.20220921 AS elemental
FROM quay.io/luet/base:$LUET_VERSION AS luet
FROM quay.io/kairos/packages:grub2-livecd-0.0.4  AS grub2
FROM quay.io/kairos/packages:grub2-efi-image-livecd-0.0.4 AS efi

FROM opensuse/leap:$LEAP_VERSION
COPY --from=elemental /usr/bin/elemental /usr/bin/elemental
COPY --from=luet /usr/bin/luet /usr/bin/luet

COPY --from=grub2 / /grub2
COPY --from=efi / /efi

RUN zypper ref && zypper dup -y
RUN zypper ref && zypper in -y xfsprogs parted util-linux-systemd e2fsprogs curl util-linux udev rsync grub2 dosfstools grub2-x86_64-efi squashfs mtools xorriso lvm2
RUN mkdir /config

COPY ./config.yaml /config/manifest.yaml
COPY ./entrypoint.sh /entrypoint.sh
COPY ./add-cloud-init.sh /add-cloud-init.sh

ENTRYPOINT [ "/entrypoint.sh" ]
