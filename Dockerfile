# BuilderHub BuildKit Operator - multi-stage, distroless, multi-arch
#
# NOTE: The primary build definition has been converted to Yamlfile (github.com/BuilderHub/Yamlfile).
# See ./Yamlfile at the repo root.
#
# Legacy: Build: docker buildx build --platform linux/amd64,linux/arm64 -t build-operator .
# Preferred: docker buildx build -f Yamlfile --platform linux/amd64,linux/arm64 -t build-operator .

# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
WORKDIR /workspace

# Install build deps
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# Build
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -ldflags="-w -s" -o manager cmd/main.go

# Runtime stage - distroless
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER nonroot:nonroot
ENTRYPOINT ["/manager"]
