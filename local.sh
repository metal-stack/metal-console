#!/usr/bin/env bash

cd cmd/bmcproxy
make ../../bin/bmc-proxy
cd ../..

cd cmd/console
make ../../bin/metal-console
cd ../..

cp bin/bmc-proxy cmd/bmcproxy/
cp bin/metal-console cmd/console/

docker-compose -f docker-compose.dev.yaml up --build
docker-compose -f docker-compose.dev.yaml down

rm -f cmd/bmcproxy/bmc-proxy
rm -f cmd/console/metal-console
