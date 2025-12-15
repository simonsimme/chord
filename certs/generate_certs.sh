#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

SHARED_DIR="$SCRIPT_DIR/shared"
CA_KEY="$SHARED_DIR/ca-key.pem"
CA_CERT="$SHARED_DIR/ca-cert.pem"

# Generate CA key and certificate if missing
if [ ! -f "$CA_KEY" ] || [ ! -f "$CA_CERT" ]; then
    echo "CA key or certificate not found in $SHARED_DIR"
    echo "Generating new CA key and certificate in $SHARED_DIR"
    openssl genrsa -out "$CA_KEY" 4096
    openssl req -x509 -new -nodes -key "$CA_KEY" -sha256 -days 3650 \
        -subj "/C=SE/ST=Stockholm/L=Stockholm/O=ChordCA/CN=ChordCA" \
        -out "$CA_CERT"
else
    echo "Using existing CA key and certificate in $SHARED_DIR"
fi

# Generate server private key
openssl genrsa -out server-key.pem 4096

# Get public and private IPs - with fallback
PUBLIC_IP=$(curl -s --connect-timeout 2 https://api.ipify.org || \
           curl -s --connect-timeout 2 http://ifconfig.me || \
           curl -s --connect-timeout 2 http://169.254.169.254/latest/meta-data/public-ipv4 || \
           echo "")

PRIVATE_IP=$(curl -s --connect-timeout 2 http://169.254.169.254/latest/meta-data/local-ipv4 2>/dev/null || echo "")

if [ -z "$PUBLIC_IP" ]; then
    PUBLIC_IP=$(ip addr show | grep "inet " | grep -v "127.0.0.1" | awk '{print $2}' | cut -d/ -f1 | head -1)
fi

if [ -z "$PRIVATE_IP" ]; then
    PRIVATE_IP=$(ip addr show | grep "inet " | grep -v "127.0.0.1" | awk '{print $2}' | cut -d/ -f1 | head -1)
fi

PUBLIC_IP=${PUBLIC_IP:-localhost}
PRIVATE_IP=${PRIVATE_IP:-127.0.0.1}

echo "Public IP: $PUBLIC_IP"
echo "Private IP: $PRIVATE_IP"
echo "Using CA cert: $CA_CERT"
echo "Using CA key: $CA_KEY"

rm -f server-csr.pem server-ext.cnf server-cert.pem

cat > server-ext.cnf <<EOF
subjectAltName = DNS:localhost,IP:127.0.0.1,IP:$PUBLIC_IP,IP:$PRIVATE_IP
EOF

openssl req -new -key server-key.pem -out server-csr.pem \
    -subj "/C=SE/ST=Stockholm/L=Stockholm/O=Chord/CN=$PUBLIC_IP"

openssl x509 -req -days 365 -in server-csr.pem \
    -CA "$CA_CERT" -CAkey "$CA_KEY" -CAcreateserial \
    -out server-cert.pem \
    -extfile server-ext.cnf

echo ""
echo "Certificate SANs:"
openssl x509 -in server-cert.pem -text -noout | grep -A1 "Subject Alternative Name"

rm -f server-ext.cnf server-csr.pem
echo "Certificates generated successfully in $SCRIPT_DIR"