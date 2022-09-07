#!/bin/bash

TEMP=$(echo $RANDOM | md5sum | head -c 20; echo)
mkdir "$TEMP"

cp config/kustomization.yaml "$TEMP"/kustomization.yaml
cp config/config.local.yaml "$TEMP"/config.yaml
cp config/creds.json "$TEMP"/creds.json

result=$(kustomize build "$TEMP")
echo "$result"
rm -rf "$TEMP"

