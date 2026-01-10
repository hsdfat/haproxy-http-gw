# Multi-stage Dockerfile for production-grade EIR service

# Stage 1: Build
FROM hsdfat/ubi8-go:1.25.2 AS builder
ENV WORKDIR=/app
# Set working directory
WORKDIR ${WORKDIR}

RUN \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=bind,target=$WORKDIR \
    go build -o /tmp/main ./cmd/eir-gateway

FROM hsdfat/haproxy:3.3.0

USER root
COPY --from=builder /tmp/main /usr/local/bin/main
RUN mkdir -p /etc/haproxy/certs /var/run /tmp/haproxy-gateway

COPY --chown=haproxy:haproxy misc/haproxy-init.cfg /etc/haproxy/haproxy.cfg
COPY --chown=haproxy:haproxy misc/gateway-entrypoint.sh /entrypoint.sh

RUN chmod +x /entrypoint.sh \
    && chown -R haproxy:haproxy /usr/local/etc/haproxy /var/run /tmp/haproxy-gateway /etc/haproxy /usr/local/bin/main \
    && chmod 775 /usr/local/etc/haproxy \
    && chmod 664 /etc/haproxy/haproxy.cfg

USER haproxy
CMD ["/entrypoint.sh"]