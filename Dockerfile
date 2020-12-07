# syntax = docker/dockerfile:experimental

FROM docker.io/golang:1.15 as build

WORKDIR /app
ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN --mount=type=cache,target=$GOPATH/pkg/mod \
    go mod download

COPY . .

RUN --mount=type=cache,target=$HOME/.cache/go-build \
    make test

ARG VERSION="none"
RUN --mount=type=cache,target=$HOME/.cache/go-build \
    make build

FROM gcr.io/distroless/static:nonroot

COPY --from=build /app/crossplane-service-broker /usr/local/bin/

ENTRYPOINT [ "/usr/local/bin/crossplane-service-broker" ]
