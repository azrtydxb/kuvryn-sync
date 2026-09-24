# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.26 AS builder
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the Go source (relies on .dockerignore to filter)
COPY . .

# Build
# the GOARCH has no default value to allow the binary to be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
# VERSION is embedded in the binary; `solder version` and the manager's
# startup log print it. It is declared here so a new version does not
# invalidate the module download layer.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a \
    -ldflags "-X github.com/azrtydxb/solder/internal/version.Version=${VERSION}" \
    -o manager cmd/main.go

# Git, Kustomize, and Helm all run in process, so the runtime needs only CA
# certificates and a writable /tmp for the source cache.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && mkdir -p /tmp && chmod 1777 /tmp
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
