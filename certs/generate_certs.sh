#!/bin/bash

# Generate CA private key and certificate
openssl genrsa -out ca-key.pem 4096
openssl req -new -x509 -days 365 -key ca-key.pem -out ca-cert.pem \
    -subj "/C=SE/ST=Stockholm/L=Stockholm/O=Chord/CN=ChordCA"

# Generate server private key
openssl genrsa -out server-key.pem 4096

# Detect all local IP addresses (compatible with different systems)
LOCAL_IPS=$(ip addr show | grep 'inet ' | grep -v '127.0.0.1' | awk '{print $2}' | cut -d'/' -f1)

# Create configuration file with all detected IPs
cat > server-ext.cnf <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no

[req_distinguished_name]
C = SE
ST = Stockholm
L = Stockholm
O = Chord
CN = 192.168.1.190

[v3_req]
keyUsage = keyEncipherment, dataEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
EOF

# Add all detected IPs
counter=1
for ip in $LOCAL_IPS; do
    echo "IP.$counter = $ip" >> server-ext.cnf
    counter=$((counter + 1))
done

# Generate server certificate signing request
openssl req -new -key server-key.pem -out server-csr.pem \
    -config server-ext.cnf

# Sign server certificate with CA
openssl x509 -req -days 365 -in server-csr.pem \
    -CA ca-cert.pem -CAkey ca-key.pem -CAcreateserial \
    -out server-cert.pem \
    -extensions v3_req -extfile server-ext.cnf

# Display the certificate to verify SANs
echo ""
echo "Certificate generated with the following IP addresses:"
openssl x509 -in server-cert.pem -text -noout | grep -A1 "Subject Alternative Name"

# Clean up
rm server-ext.cnf server-csr.pem

echo ""
echo "Certificates generated successfully in current directory"