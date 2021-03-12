#!/bin/sh

set -e -x

export GOPATH=$(pwd)/gopath

cd gopath/src/github.com/alphagov/paas-cdn-broker
ginkgo -r
