#!/bin/bash

set -eux

export GOPATH=$(pwd)/gopath
export PATH=${PATH}:${GOPATH}/bin
mkdir -p ${GOPATH}/bin

curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

pushd gopath/src/github.com/18F/cf-cdn-service-broker
  dep ensure
  go test $(go list ./... | grep -v /vendor/)
popd
