#!/bin/bash

# Change to the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Generate CA private key and certificate (run once, share across instances)
openssl genrsa -out ca-key.pem 4096
openssl req -new -x509 -days 365 -key ca-key.pem -out ca-cert.pem \
    -subj "/C=SE/ST=Stockholm/L=Stockholm/O=Chord/CN=ChordCA"

# Generate server private key
openssl genrsa -out server-key.pem 4096

# Get public and private IPs - with fallback
PUBLIC_IP=$(curl -s --connect-timeout 1 http://169.254.169.254/latest/meta-data/public-ipv4 2>/dev/null || echo "")
PRIVATE_IP=$(curl -s --connect-timeout 1 http://169.254.169.254/latest/meta-data/local-ipv4 2>/dev/null || echo "")

# If still empty, use alternative methods
if [ -z "$PUBLIC_IP" ]; then
    PUBLIC_IP=$(ip addr show | grep "inet " | grep -v "127.0.0.1" | awk '{print $2}' | cut -d/ -f1 | head -1)
fi

if [ -z "$PRIVATE_IP" ]; then
    PRIVATE_IP=$(ip addr show | grep "inet " | grep -v "127.0.0.1" | awk '{print $2}' | cut -d/ -f1 | head -1)
fi

# Fallback to localhost if nothing found
PUBLIC_IP=${PUBLIC_IP:-localhost}
PRIVATE_IP=${PRIVATE_IP:-127.0.0.1}

echo "Public IP: $PUBLIC_IP"
echo "Private IP: $PRIVATE_IP"

# Clean up old files
rm -f server-csr.pem server-ext.cnf server-cert.pem

# Create configuration with both public and private IPs
cat > server-ext.cnf <<EOF
subjectAltName = DNS:localhost,IP:127.0.0.1,IP:$PUBLIC_IP,IP:$PRIVATE_IP
EOF

# Generate and sign certificate
openssl req -new -key server-key.pem -out server-csr.pem \
    -subj "/C=SE/ST=Stockholm/L=Stockholm/O=Chord/CN=$PUBLIC_IP"

openssl x509 -req -days 365 -in server-csr.pem \
    -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
    -out server-cert.pem \
    -extfile server-ext.cnf

echo ""
echo "Certificate SANs:"
openssl x509 -in server-cert.pem -text -noout | grep -A1 "Subject Alternative Name"

rm -f server-ext.cnf server-csr.pem
echo "Certificates generated successfully in $SCRIPT_DIR"