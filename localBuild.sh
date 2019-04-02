#!/usr/bin/env bash

cd cmd/bmcproxy
make
cd ../console
make
cd ../..

docker-compose -f docker-compose.dev.yaml build