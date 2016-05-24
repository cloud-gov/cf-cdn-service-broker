#!/bin/sh

set -e -x

export GOPATH=$(pwd)/gopath

cd gopath/src/github.com/18F/cf-cdn-service-broker
go test $(go list ./... | grep -v /vendor/)
