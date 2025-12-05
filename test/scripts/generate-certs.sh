#!/bin/bash

set -e

CERT_DIR="${1:-./test/certs}"

echo "Generating self-signed certificates in $CERT_DIR"

mkdir -p "$CERT_DIR"

# Generate CA
openssl genrsa -out "$CERT_DIR/ca.key" 4096
openssl req -new -x509 -days 365 -key "$CERT_DIR/ca.key" -out "$CERT_DIR/ca.crt" -subj "/C=US/ST=Test/L=Test/O=Test/CN=Test CA"

# Generate server certificate
openssl genrsa -out "$CERT_DIR/server.key" 2048
openssl req -new -key "$CERT_DIR/server.key" -out "$CERT_DIR/server.csr" -subj "/C=US/ST=Test/L=Test/O=Test/CN=*.example.com"

cat > "$CERT_DIR/server.ext" << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = *.example.com
DNS.2 = api.example.com
DNS.3 = www.example.com
DNS.4 = localhost
EOF

openssl x509 -req -in "$CERT_DIR/server.csr" -CA "$CERT_DIR/ca.crt" -CAkey "$CERT_DIR/ca.key" -CAcreateserial \
    -out "$CERT_DIR/server.crt" -days 365 -sha256 -extfile "$CERT_DIR/server.ext"

# Combine for HAProxy
cat "$CERT_DIR/server.crt" "$CERT_DIR/server.key" > "$CERT_DIR/server.pem"

echo "Certificates generated successfully!"
echo "  CA Certificate: $CERT_DIR/ca.crt"
echo "  Server Certificate: $CERT_DIR/server.crt"
echo "  Server Key: $CERT_DIR/server.key"
echo "  Combined PEM: $CERT_DIR/server.pem"
