#!/bin/bash

set -eux

export GOPATH=$(pwd)/gopath
export PATH=${PATH}:${GOPATH}/bin
mkdir -p ${GOPATH}/bin

pushd gopath/src/github.com/cloud-gov/cf-cdn-service-broker
  go test $(go list ./... | grep -v /vendor/)
popd
