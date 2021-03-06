#!/bin/bash

set -eux

cd "`dirname $0`/.."

: ${GOPATH:=${WORKSPACE}/.gopath}
if [[ "${GOPATH}" != *:* ]]; then
  mkdir -p "${GOPATH}"
  export PATH="${GOPATH}/bin:${PATH}"
fi
export GOPATH

go get github.com/constabulary/gb/...
gb generate github.com/matrix-org/dendron/proxy
gb build
