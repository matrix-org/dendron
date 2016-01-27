#! /bin/bash

set -e

for dir in $(find src/* -type d); do golint $dir/*.go; done

go tool vet src/

gb test
