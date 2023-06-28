#!/bin/bash
# usage:
# docker run --rm -ti --entrypoint /update-os-release.sh \
# -v /etc:/workspace \ # mount the directory where your os-release is, this is by default in /etc but you can mount a different dir for testing
# -e OS_NAME=kairos-core-opensuse-leap \
# -e OS_VERSION=v2.2.0 \
# -e OS_ID="kairos" \
# -e OS_NAME=kairos-core-opensuse-leap \
# -e BUG_REPORT_URL="https://github.com/kairos-io/kairos/issues" \
# -e HOME_URL="https://github.com/kairos-io/kairos" \
# -e OS_REPO="quay.io/kairos/core-opensuse-leap" \
# -e OS_LABEL="latest" \
# -e GITHUB_REPO="kairos-io/kairos" \
# -e VARIANT="core" \
# -e FLAVOR="opensuse-leap"
# quay.io/kairos/osbuilder-tools:latest

set -ex

sed -i -n '/KAIROS_/!p' /workspace/os-release
envsubst >>/workspace/os-release < /os-release.tmpl

cat /workspace/os-release
