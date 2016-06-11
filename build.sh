#!/bin/sh
set -e

go fmt

CGO_ENABLED=0 go build -a

docker build -t .
# docker push mopsalarm/go-pr0gramm-meta-update
