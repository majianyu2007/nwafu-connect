# syntax=docker/dockerfile:1
# get modules, if they don't change the cache can be used for faster builds
FROM --platform=$BUILDPLATFORM golang:1.25 AS base
ENV GO111MODULE=on
ENV CGO_ENABLED=0
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT_ID
# RUN go env -w GOPROXY=https://goproxy.cn,direct

WORKDIR /src
COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# build the application
FROM base AS build
RUN --mount=target=. \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -v -o /app/nwafu-connect -trimpath \
    -ldflags "-s -w -buildid= -X main.nwafuConnectVersion=${VERSION} -X main.CommitID=${COMMIT_ID}" .

# Import the binary from build stage
# use root container, but still use /home/nonroot to keep backward support
FROM gcr.io/distroless/static AS prd
WORKDIR /home/nonroot
COPY --from=build /app/nwafu-connect /home/nonroot
ENTRYPOINT ["/home/nonroot/nwafu-connect", "-config", "/home/nonroot/config.toml"]
