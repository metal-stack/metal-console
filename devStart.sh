#!/usr/bin/env bash

if [[ -z ${METALCTL_HMAC} ]]; then
  echo "env var METALCTL_HMAC not set"
  exit 1
fi

docker-compose up
docker-compose down
