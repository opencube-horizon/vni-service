.PHONY: main.go

TAG ?= 1.2.1

all: main

main: src/main.go
	buildah build \
		--layers \
		--tag vni_service_endpoint:$(TAG) \
		--env VERSION="$(TAG)" \
		--env TIMESTAMP="$$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
		--compress \
		--squash \
		.