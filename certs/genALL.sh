#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHARED_DIR="$SCRIPT_DIR/shared"
CA_KEY="$SHARED_DIR/ca-key.pem"
CA_CERT="$SHARED_DIR/ca-cert.pem"

mkdir -p "$SHARED_DIR"

if [ -f "$CA_KEY" ] || [ -f "$CA_CERT" ]; then
    echo "ERROR: ca-key.pem or ca-cert.pem already exists in $SHARED_DIR"
    echo "Remove them first if you want to generate new CA files."
    exit 1
fi

echo "Generating new CA key and certificate in $SHARED_DIR"
openssl genrsa -out "$CA_KEY" 4096
openssl req -x509 -new -nodes -key "$CA_KEY" -sha256 -days 3650 \
    -subj "/C=SE/ST=Stockholm/L=Stockholm/O=ChordCA/CN=ChordCA" \
    -out "$CA_CERT"

echo "CA key and certificate generated in $SHARED_DIR"