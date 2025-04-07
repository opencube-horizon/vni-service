#!/usr/bin/bash
go build \
  -ldflags '-linkmode external -s -w -extldflags "--static"' \
  -tags 'osusergo,netgo,static_build,sqlite_vtable'

get_pods() {
  /opt/k3s/k3s-arm64 kubectl -n vni-management get pods -l app="vni-endpoint" --no-headers|awk '{print $1}'
}

buildah build -t vni_service_endpoint --layers --compress -f Containerfile

podman image push localhost/vni_service_endpoint:latest \
  aam1.caps.cit.tum.de:9443/vni-service-endpoint:latest

/opt/k3s/k3s-arm64 kubectl -n vni-management delete pod -- $(get_pods)

sleep 1

/opt/k3s/k3s-arm64 kubectl -n vni-management logs --follow --  $(get_pods)