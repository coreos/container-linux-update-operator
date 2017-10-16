#!/bin/bash
set -euo pipefail
set -x

readonly REPO_ROOT=$(git rev-parse --show-toplevel)

sudo rkt run --uuid-file-save=rkt.uuid \
    --volume repo-root,kind=host,source=${REPO_ROOT} \
    --mount volume=repo-root,target=/go/src/github.com/coreos/container-linux-update-operator \
    --insecure-options=image,ondisk docker://golang:1.8.4 --exec /bin/bash -- -c \
    "cd /go/src/github.com/coreos/container-linux-update-operator && make clean test all"

sudo rkt rm --uuid-file=rkt.uuid
rm -f rkt.uuid
