FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG REPO_URL
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "-s -w -X allstar-yaamon/internal/config.DefaultFooterURL=${REPO_URL}" -o yaamon .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl && \
    adduser -D -s /sbin/nologin -u 1000 yaamon && \
    mkdir -p /etc/yaamon /var/lib/yaamon && \
    chown yaamon:yaamon /etc/yaamon /var/lib/yaamon
COPY --from=builder /build/yaamon /usr/local/bin/yaamon
COPY docker-entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh
USER yaamon
WORKDIR /etc/yaamon
EXPOSE 80 443
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s \
    CMD curl -sf http://localhost:80/health || exit 1
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["/usr/local/bin/yaamon", "serve"]
