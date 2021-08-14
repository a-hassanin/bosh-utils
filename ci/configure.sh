#!/bin/bash

set -e

lpass ls > /dev/null

fly -t "${CONCOURSE_TARGET:-bosh-ecosystem}" set-pipeline -p bosh-utils -c ci/pipeline.yml \
    --load-vars-from <(lpass show -G "bosh-utils concourse secrets" --notes) \
    --load-vars-from <(lpass show --note "bosh:docker-images concourse secrets")
