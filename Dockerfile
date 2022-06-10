FROM golang:1.18 AS build
WORKDIR /go/src/github.com/cpuguy83/dockersource
COPY go.mod .
COPY go.sum .
RUN \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download
COPY . .
RUN \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build .


FROM scratch
COPY --from=build /go/src/github.com/cpuguy83/dockersource/dockersource /