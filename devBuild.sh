#!/usr/bin/env bash

set -e

make bmcproxy
make console
docker-compose build