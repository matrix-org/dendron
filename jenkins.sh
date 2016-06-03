#!/bin/bash -u

: ${GOPATH:=${WORKSPACE}/.gopath}
if [[ "${GOPATH}" != *:* ]]; then
  mkdir -p "${GOPATH}"
  export PATH="${GOPATH}/bin:${PATH}"
fi
export GOPATH

go get github.com/constabulary/gb/...
go get github.com/golang/lint/golint
go get github.com/tebeka/go2xunit

gb generate
gb build

# TODO: Whatever comes out of https://github.com/constabulary/gb/issues/559
GOPATH=$(pwd):$(pwd)/vendor go test $(gb list) -v | go2xunit > results.xml
golint src/... >golint.txt
go tool vet src/ 2>govet.txt
./scripts/check_generated_proxy_paths.sh || exit 1

set -e

: ${GIT_BRANCH:="origin/$(git rev-parse --abbrev-ref HEAD)"}
: ${WORKSPACE:="$(pwd)"}

if [[ ! -e .synapse-base ]]; then
  git clone https://github.com/matrix-org/synapse.git .synapse-base --mirror
else
  (cd .synapse-base; git fetch -p)
fi

rm -rf synapse
git clone .synapse-base synapse --shared

(cd synapse ; git checkout "${GIT_BRANCH}" 2>/dev/null || (echo >&2 "Synapse: No ref ${GIT_BRANCH} found, falling back to develop" ; git checkout develop))
virtualenv $WORKSPACE/.venv
PYTHON=$WORKSPACE/.venv/bin/python
PIP=$WORKSPACE/.venv/bin/pip
(cd synapse ; $PYTHON synapse/python_dependencies.py | xargs -n1 $PIP install)
$PIP install psycopg2
$PIP install lxml

if [[ ! -e .sytest-base ]]; then
  git clone https://github.com/matrix-org/sytest.git .sytest-base --mirror
else
  (cd .sytest-base; git fetch -p)
fi

rm -rf sytest
git clone .sytest-base sytest --shared
cd sytest

git checkout "${GIT_BRANCH}" 2>/dev/null || (echo >&2 "Sytest: No ref ${GIT_BRANCH} found, falling back to develop" ; git checkout develop)


: ${PERL5LIB:=$WORKSPACE/perl5/lib/perl5}
: ${PERL_MB_OPT:=--install_base=$WORKSPACE/perl5}
: ${PERL_MM_OPT:=INSTALL_BASE=$WORKSPACE/perl5}
export PERL5LIB PERL_MB_OPT PERL_MM_OPT

./install-deps.pl

: ${PORT_BASE:=8000}

: PGUSER=${PGUSER:=$USER}
: PGPASSWORD=${PGPASSWORD:=}
export PGUSER PGPASSWORD

RUN_POSTGRES=""
for port in $(($PORT_BASE + 1)) $(($PORT_BASE + 2)); do
    if psql sytest_jenkins_$port -h localhost <<< ""; then
        mkdir -p $WORKSPACE/sytest/localhost-$port
        RUN_POSTGRES=$RUN_POSTGRES:$port
	readlink -f $WORKSPACE/sytest/localhost-$port/database.yaml
        cat > $WORKSPACE/sytest/localhost-$port/database.yaml << EOF
name: psycopg2
args:
    database: sytest_jenkins_$port
    host: localhost
    user: ${PGUSER}
    password: ${PGPASSWORD}
    sslmode: disable
EOF
    fi
done

# Run if both postgresql databases exist
if test "$RUN_POSTGRES" = ":$(($PORT_BASE + 1)):$(($PORT_BASE + 2))"; then
    echo >&2 "Running sytest with PostgreSQL";
    $PIP install psycopg2
    ./run-tests.pl \
      -O tap \
      --synapse-directory=$WORKSPACE/synapse \
      --dendron=$WORKSPACE/bin/dendron \
      --python=$PYTHON \
      --all \
      --port-base=$PORT_BASE > results.tap
else
    echo >&2 "Skipping running sytest with PostgreSQL, $RUN_POSTGRES"
fi
