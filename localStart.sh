#!/usr/bin/env bash

docker-compose -f docker-compose.dev.yaml up
docker-compose -f docker-compose.dev.yaml down
