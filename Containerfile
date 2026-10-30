FROM golang:1.27-alpine as builder

WORKDIR /usr/src
RUN apk add --no-cache musl-dev build-base
COPY src/ /usr/src

ARG VERSION=""
ARG TIMESTAMP=""
RUN go mod tidy \
 && go build \
 	-ldflags "-linkmode external -s -w -extldflags '--static' -X main.version=${VERSION} -X main.buildTime=${TIMESTAMP}" \
	-tags 'osusergo,netgo,static_build,sqlite_vtable'

FROM alpine
COPY --from=builder /usr/src/vni_service /usr/local/bin/vni_service
RUN apk upgrade --no-cache && apk add --no-cache 'libssl3' 'libcrypto3' 'sqlite'
RUN adduser --uid 1022 --disabled-password --system --no-create-home vniservice

USER 1022:1022

ENTRYPOINT ["/usr/local/bin/vni_service"]
CMD ["--file", "/opt/db/db.sqlite3", "--vni_allocs_log", "--log_level", "info"]
EXPOSE 8842