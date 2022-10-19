#!/bin/bash

partprobe

kpartx -va $DRIVE

image=$1

if [ -z "$image" ]; then
    echo "No image specified"
    exit 1
fi

set -ax
TEMPDIR="$(mktemp -d)"
echo $TEMPDIR
mount "${device}p1" "${TEMPDIR}"

for dir in /rpi/u-boot /rpi/rpi-firmware /rpi/rpi-firmware-config /rpi/rpi-firmware-dt
do
    cp -rfv ${dir}/* $TEMPDIR
done

umount "${TEMPDIR}"
