#!/bin/bash

set -eux

pushd gopath/src/github.com/18F/cf-cdn-service-broker
  go get -v ./...
  go test $(go list ./... | grep -v /vendor/)
popd
