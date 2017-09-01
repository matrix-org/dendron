#!/bin/bash -u

cd "$(dirname "$0")"/..

diff="$(diff -u src/github.com/matrix-org/dendron/proxy/paths.go <(python scripts/generate_proxy_paths.py vendor/src/github.com/matrix-org/matrix-doc /dev/stdout))"
if [[ $? != 0 ]]; then
  cat >&2 <<EOF
src/github.com/matrix-org/dendron/proxy/paths.go is out of date, you should regenerate it. It will apply the following diff:

${diff}
EOF
  exit 1
fi
