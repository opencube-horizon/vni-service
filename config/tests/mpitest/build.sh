#!/usr/bin/bash

NAME=mpitest
TAG=0.1

buildah build \
       --layers --tls-verify=false \
        --registries-conf /etc/containers/registries.conf \
        --tag $NAME:$TAG \
        --compress \
        --format docker \
        --squash \
        --file Dockerfile

podman image push \
  localhost/$NAME:$TAG \
  aam1.caps.cit.tum.de:9443/$NAME:$TAG