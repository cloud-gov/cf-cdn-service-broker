#!/bin/bash

set -eux

pushd cf-cdn-service-broker
  go mod download
  go mod vendor
  go test -v ./...
popd
