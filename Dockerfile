# syntax=docker/dockerfile:1.7
#
# Public backend-only image. /ui/ is unavailable (-tags noui), so no proprietary
# NumSpot components/fonts or private GitLab access enter this build or its cache.
# For authorized internal UI-enabled images, use `make docker-build-ui`.
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .
ARG VERSION=docker
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOARCH=${TARGETARCH:-amd64} \
    go build \
        -tags noui \
        -trimpath \
        -ldflags "-s -w -X main.version=${VERSION}" \
        -o /out/longue-vue \
        ./cmd/longue-vue

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/longue-vue /longue-vue
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/longue-vue"]
