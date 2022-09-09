#!/usr/bin/env bash

NEW_TAG="$1"

echo "New tag detected: $NEW_TAG"
# make temp directory for kustomization file work
DIR=$(mktemp -d)

# copy current release deployment yaml file into temp directory
cp deploy/release.yaml "$DIR"/release.yaml

# saves the current working directory in memory so it can be returned to at any time, and moves to the parent directory
pushd "$DIR" || exit

# create kustomization file to dynamically replace the image:tag
cat <<-EOF | envsubst - > kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
 - "release.yaml"
images:
 - name: ghcr.io/ahanafy/kubesync/kubesync
   newName: ghcr.io/ahanafy/kubesync/kubesync
   newTag: $NEW_TAG
EOF


kustomize build . > _newrelease.yaml

# returns to the path at the top of the directory stack
popd || exit

# update release.yaml with new image:tag
cp "$DIR"/_newrelease.yaml deploy/release.yaml

# remove temp dir
#rm -rf "$DIR"
echo "$DIR"
