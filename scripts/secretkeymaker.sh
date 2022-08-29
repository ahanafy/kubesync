#!/bin/bash
PRIVATEKEY=customtls.key
PUBLICKEY=customtls.crt
NAMESPACE=default
SECRETNAME=custom-keys-$(date +%s)
K8SMANIFEST=secret.yaml

echo "Executing Key Generation..."
openssl req -x509 -nodes -newkey rsa:4096 -keyout "$PRIVATEKEY" -out "$PUBLICKEY" -subj "/CN=sealed-secret/O=sealed-secret"

kubectl -n "$NAMESPACE" create secret tls "$SECRETNAME" --cert="$PUBLICKEY" --key="$PRIVATEKEY" --dry-run=client -oyaml > $K8SMANIFEST

yq -i '.metadata.labels["sealedsecrets.bitnami.com/sealed-secrets-key"] = "active"' $K8SMANIFEST

yq -i 'del(metadata.creationTimestamp)' $K8SMANIFEST

rm $PRIVATEKEY $PUBLICKEY