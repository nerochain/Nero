#!/bin/bash
set -e

docker build -f Dockerfile.debian -t nero-debian-client:build  .

# copy binary out of the local image
docker run -d --name nero-debian-for-copy nero-debian-client:build /bin/bash
docker cp nero-debian-for-copy:/go-ethereum/build/bin/geth ${PWD}/build/bin/geth-ubuntu
docker rm nero-debian-for-copy
ls -l ./build/bin/