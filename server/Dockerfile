# syntax=docker/dockerfile:1

# Image Linux of zerechnung-server: linux/amd64, linux/arm64 and linux/arm/v7.
# The build context is the root of the repository:
#
#	docker build -f server/Dockerfile -t zerechnung .
#
# The binaries are static (CGO_ENABLED=0) and cross-compiled on the platform
# of the builder, so that a multi-platform build needs no emulation:
#
#	docker buildx build -f server/Dockerfile --platform linux/amd64,linux/arm64,linux/arm/v7 .

ARG GO_VERSION=1.27

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY server/go.mod server/
COPY . .
ARG TARGETOS TARGETARCH TARGETVARIANT
ARG VERSION=dev
ENV CGO_ENABLED=0 GOFLAGS="-trimpath -buildvcs=false"
RUN --mount=type=cache,target=/root/.cache/go-build \
    export GOOS="$TARGETOS" GOARCH="$TARGETARCH" GOARM="${TARGETVARIANT#v}" && \
    mkdir -p /out && \
    go build -ldflags "-s -w -X main.version=$VERSION" -o /out/zerechnung ./cmd/zerechnung && \
    cd server && \
    go build -ldflags "-s -w -X main.version=$VERSION" -o /out/zerechnung-server ./cmd/zerechnung-server

# distroless/static: the CA certificates, the time zones and a nonroot user
# (65532), without shell nor package manager.
FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
LABEL org.opencontainers.image.title="zerechnung" \
      org.opencontainers.image.description="HTTP API that creates, signs, validates and converts Factur-X, ZUGFeRD, XRechnung, UBL, FatturaPA and Facturae e-invoices" \
      org.opencontainers.image.source="https://github.com/ritoon/zerechnung" \
      org.opencontainers.image.documentation="https://github.com/ritoon/zerechnung/tree/main/server" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="$VERSION"
COPY --from=build /out/zerechnung-server /out/zerechnung /usr/local/bin/
# The licenses: the project's, and the Go fonts' embedded in the PDFs.
COPY LICENSE /usr/share/doc/zerechnung/LICENSE
COPY server/internal/render/fonts/LICENSE /usr/share/doc/zerechnung/fonts/LICENSE
USER nonroot:nonroot
ENV ZR_PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/usr/local/bin/zerechnung-server", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/zerechnung-server"]
CMD ["serve"]
