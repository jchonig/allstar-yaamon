FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "-s -w" -o yaamon .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata curl && \
    adduser -D -s /sbin/nologin -u 1000 yaamon && \
    mkdir -p /etc/yaamon /data && \
    chown yaamon:yaamon /etc/yaamon /data
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
