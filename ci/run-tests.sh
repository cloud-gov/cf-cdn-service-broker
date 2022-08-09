#!/bin/sh

set -e -x

cd gopath/src/github.com/alphagov/paas-cdn-broker
ginkgo -r
