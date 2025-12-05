#!/bin/sh

set -e

echo "Starting HAProxy HTTP Gateway..."

# Wait for HAProxy to be available
until [ -S /var/run/haproxy-runtime-api.sock ]; do
    echo "Waiting for HAProxy runtime socket..."
    sleep 1
done

echo "HAProxy is ready, starting HTTP Gateway application..."

# Start the HTTP Gateway
exec /usr/local/bin/http-gateway
