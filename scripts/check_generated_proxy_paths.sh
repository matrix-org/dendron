#!/bin/bash -eu

cd "$(dirname "$(dirname "$(realpath "$0")")")"

rm -rf matrix-doc
git clone https://github.com/matrix-org/matrix-doc.git
trap "rm -rf matrix-doc" EXIT

set +e
diff="$(diff -u src/github.com/matrix-org/dendron/proxy/paths.go <(python scripts/generate_proxy_paths.py matrix-doc /dev/stdout))"
if [[ $? != 0 ]]; then
  cat >&2 <<EOF
src/github.com/matrix-org/dendron/proxy/paths.go is out of date, you should regenerate it. It will apply the following diff:

${diff}
EOF
  exit 1
fi
