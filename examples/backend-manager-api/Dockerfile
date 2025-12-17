FROM golang:1.24-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the main gateway
RUN CGO_ENABLED=0 GOOS=linux go build -o /http-gateway ./cmd/http-gateway

# Build the backend manager API
RUN CGO_ENABLED=0 GOOS=linux go build -o /backend-manager-api ./examples/backend-manager-api/main.go

# Final stage - based on HAProxy image
FROM haproxytech/haproxy-alpine:3.2

RUN apk add --no-cache socat openssl ca-certificates

# Copy gateway binary
COPY --from=builder /http-gateway /usr/local/bin/http-gateway
RUN chmod +x /usr/local/bin/http-gateway

# Copy backend manager API binary
COPY --from=builder /backend-manager-api /usr/local/bin/backend-manager-api
RUN chmod +x /usr/local/bin/backend-manager-api

# Create directories
RUN mkdir -p /etc/haproxy/certs \
    /var/run \
    /tmp/haproxy-gateway

# Copy initial HAProxy config and entrypoint
COPY --chown=haproxy:haproxy test/haproxy-init.cfg /etc/haproxy/haproxy.cfg
COPY --chown=haproxy:haproxy test/scripts/gateway-entrypoint.sh /entrypoint.sh

# Set permissions and ownership
RUN chmod +x /entrypoint.sh \
    && chmod 644 /etc/haproxy/haproxy.cfg \
    && chmod 775 /usr/local/etc/haproxy \
    && chown -R haproxy:haproxy /usr/local/etc/haproxy /var/run /tmp/haproxy-gateway

USER haproxy
EXPOSE 8080 8443 9090 9091

ENTRYPOINT ["/entrypoint.sh"]
