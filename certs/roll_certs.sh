#!/usr/bin/env bash

set -eo pipefail

rm -f *.pem
cfssl gencert -ca=../ca.pem -ca-key=../ca-key.pem -config=../ca-config.json -profile=server server.json | cfssljson -bare server
cfssl gencert -ca=../ca.pem -ca-key=../ca-key.pem -config=../ca-config.json -profile=client client.json | cfssljson -bare client
rm *.csr
./test_certs.sh --client-pem=client.pem --client-key=client-key.pem --server-pem=server.pem --server-key=server-key.pem --host=metal-console
ssh-keygen -y -f server-key.pem > server-key.pub
