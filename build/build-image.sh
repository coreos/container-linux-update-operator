#!/bin/bash
set -euo pipefail
set -x

IMAGE_REPO=${IMAGE_REPO:-quay.io/coreos/container-linux-update-operator}

readonly REPO_ROOT=$(git rev-parse --show-toplevel)
readonly VERSION=${VERSION:-$(${REPO_ROOT}/build/git-version.sh)}

sudo rkt run --uuid-file-save=rkt.uuid \
    --volume repo-root,kind=host,source=${REPO_ROOT} \
    --mount volume=repo-root,target=/go/src/github.com/coreos/container-linux-update-operator \
    --insecure-options=image,ondisk docker://golang:1.8.4 --exec /bin/bash -- -c \
    "cd /go/src/github.com/coreos/container-linux-update-operator && make clean test all"

sudo rkt rm --uuid-file=rkt.uuid
rm -f rkt.uuid

cd ${REPO_ROOT} && docker build -t ${IMAGE_REPO}:${VERSION} .

if [[ ${PUSH_IMAGE:-false} == "true" ]]; then
    docker push ${IMAGE_REPO}:${VERSION}
fi
