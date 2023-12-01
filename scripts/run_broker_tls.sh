#!/usr/bin/env bash

set -euo pipefail

_tmp_dir="$(mktemp -d)"
#trap 'rm -rf "${_tmp_dir}"' EXIT

pushd "${_tmp_dir}" > /dev/null
    echo "Generating TLS Certificates"
    # Generate the CA Key and Certificate for signing Client Certs
    openssl req -x509 -newkey rsa:1024 -nodes \
        -keyout CAkey.pem -out CAcert.pem \
        -sha256 -days 3650 -nodes \
        -subj "/C=UK/ST=London/L=London/O=PAASDEV/CN=broker-ca.local" &> /dev/null

    # Generate the Server Key, and CSR and sign with the CA Certificate
    openssl req -new -newkey rsa:1024 -nodes \
        -keyout key.pem -out req.pem \
        -subj "/C=UK/ST=London/L=London/O=PAASDEV/CN=broker.local" &> /dev/null
    openssl x509 -req \
        -in req.pem -CA CAcert.pem -CAkey CAkey.pem \
        -extfile <(printf "subjectAltName=IP:127.0.0.1") \
        -CAcreateserial -out cert.pem -days 30 -sha256 #&> /dev/null

    # rm CAkey.pem req.pem

    cert_value="$(sed -e ':a' -e 'N' -e '$!ba' -e 's/\n/\\n/g' "cert.pem")"
    key_value="$(sed -e ':a' -e 'N' -e '$!ba' -e 's/\n/\\n/g' "key.pem")"
    ca_cert_value="$(sed -e ':a' -e 'N' -e '$!ba' -e 's/\n/\\n/g' "CAcert.pem")"
popd > /dev/null

echo "TLS Certificates Generated in ${_tmp_dir}"
echo "Example curl command: curl --cacert ${_tmp_dir}/CAcert.pem https://127.0.0.1:3000"

jq \
    '.tls.certificate = "'"${cert_value}"'" | .tls.private_key = "'"${key_value}"'" | .tls.ca = "'"${ca_cert_value}"'"' \
    ./examples/config.json > "${_tmp_dir}/config.json"
#go run main.go --config "${_tmp_dir}/config.json"
