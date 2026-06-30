ARG GO_VERSION=1.26.0
ARG ALPINE_VERSION=3.22

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY build ./build
COPY cmd ./cmd
COPY config ./config
COPY embed ./embed
COPY internal ./internal
COPY main.go doc.go README.md ./

RUN go run ./build/build.go && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /out/superflare .

FROM alpine:${ALPINE_VERSION} AS runtime

RUN apk add --no-cache ca-certificates tzdata iproute2 net-tools

WORKDIR /data

COPY --from=builder /out/superflare /opt/superflare/superflare
COPY tools/docker/docker-entrypoint.sh /opt/superflare/docker-entrypoint.sh

RUN chmod +x /opt/superflare/superflare /opt/superflare/docker-entrypoint.sh && \
    mkdir -p /data /data/var/uploads /data/var/cache /data/var/tools /data/var/lighthouse /data/var/screens

ENV FLARE_PORT=3636

VOLUME ["/data"]
EXPOSE 3636

ENTRYPOINT ["/opt/superflare/docker-entrypoint.sh"]
CMD []
