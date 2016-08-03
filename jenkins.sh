#!/bin/bash

set -eux

./jenkins/build_dendron.sh
./jenkins/test_dendron.sh

./jenkins/clone.sh synapse https://github.com/matrix-org/synapse.git
./jenkins/clone.sh sytest https://github.com/matrix-org/synapse.git

./synapse/jenkins/prepare_synapse.sh
./sytest/jenkins/prep_sytest_for_postgres.sh

./sytest/jenkins/install_and_run.sh \
      --synapse-directory=$WORKSPACE/synapse \
      --dendron=$WORKSPACE/bin/dendron \
