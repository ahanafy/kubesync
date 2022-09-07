#!/bin/bash

RANDO=$(echo $RANDOM | md5sum | head -c 20; echo)
mkdir "$RANDO"
cat <<'EOF' >"$RANDO"/kustomization.yaml
namespace: default
secretGenerator:
  - name: config-secret
    files:
      - "./config.yaml"
    options:
      disableNameSuffixHash: true
EOF

cp config/config.local.yaml "$RANDO"/config.yaml

result=$(kustomize build "$RANDO")
echo "$result"
rm -rf "$RANDO"
