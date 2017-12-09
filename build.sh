#!/bin/sh
set -e

docker build -t mopsalarm/go-pr0gramm-meta-update .
docker push mopsalarm/go-pr0gramm-meta-update
