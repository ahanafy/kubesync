#!/bin/bash

curl -L https://www.toptal.com/developers/gitignore/api/go,visualstudiocode,git --output ".gitignore"
cat <<EOT >> .gitignore

# User supplied values

__debug_bin
*.local.yaml
*creds.json
release.yaml

# End of User supplied values
EOT

