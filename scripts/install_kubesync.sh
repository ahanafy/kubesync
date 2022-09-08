#!/usr/bin/env bash
# Copyright 2022 The Kubernetes Authors.
# SPDX-License-Identifier: Apache-2.0
# Copyright 2022 Alan Hanafy.

# If no argument is given -> Downloads the most recently released
# kubesync deployment manifest to your current working directory.
# (e.g. 'install_kubesync.sh')
#
# If one argument is given -> 
# If that argument is in the format of #.#.#, downloads the specified
# version of the kubesync deployment manifest to your current working directory.
# If that argument is something else, downloads the most recently released
# kubesync deployment manifest to the specified directory.
# (e.g. 'install_kubesync.sh 3.8.2' or 'install_kubesync.sh ./build')
#
# If two arguments are given -> Downloads the specified version of the
# kubesync deployment manifest to the specified directory.
# (e.g. 'install_kubesync.sh 3.8.2 ./build
#
# Fails if the file already exists.

set -e

# Unset CDPATH to restore default cd behavior. An exported CDPATH can
# cause cd to output the current directory to STDOUT.
unset CDPATH

where=$PWD

release_url=https://api.github.com/repos/ahanafy/kubesync/releases

if [ -n "$1" ]; then
  if [[ "$1" =~ ^[0-9]+(\.[0-9]+){2}$ ]]; then
    version=v$1
    release_url=${release_url}/tags/$version
  elif [ -n "$2" ]; then
    echo "The first argument should be the requested version."
    exit 1
  else
    where="$1"
  fi
fi

if [ -n "$2" ]; then
  where="$2"
fi

if ! test -d "$where"; then
  echo "$where does not exist. Create it first."
  exit 1
fi

# Emulates `readlink -f` behavior, as this is not available by default on MacOS
# See: https://stackoverflow.com/questions/1055671/how-can-i-get-the-behavior-of-gnus-readlink-f-on-a-mac
function readlink_f {
  TARGET_FILE=$1

  cd "$(dirname "$TARGET_FILE")"
  TARGET_FILE=$(basename "$TARGET_FILE")

  # Iterate down a (possible) chain of symlinks
  while [ -L "$TARGET_FILE" ]
  do
      TARGET_FILE=$(readlink "$TARGET_FILE")
      cd "$(dirname "$TARGET_FILE")"
      TARGET_FILE=$(readlink "$TARGET_FILE")
  done

  # Compute the canonicalized name by finding the physical path
  # for the directory we're in and appending the target file.
  PHYS_DIR=$(pwd -P)
  RESULT=$PHYS_DIR/$TARGET_FILE
  echo "$RESULT"
}

where="$(readlink_f "$where")/"

if [ -f "${where}release.yaml" ]; then
  echo "${where}release.yaml exists. Remove it first."
  exit 1
elif [ -d "${where}release.yaml" ]; then
  echo "${where}release.yaml exists and is a directory. Remove it first."
  exit 1
fi

tmpDir=$(mktemp -d)
if [[ ! "$tmpDir" || ! -d "$tmpDir" ]]; then
  echo "Could not create temp dir."
  exit 1
fi

function cleanup {
  rm -rf "$tmpDir"
}

trap cleanup EXIT ERR

pushd "$tmpDir" >& /dev/null

releases=$(curl -s "$release_url")

if [[ $releases == *"API rate limit exceeded"* ]]; then
  echo "Github rate-limiter failed the request. Either authenticate or wait a couple of minutes."
  exit 1
fi

RELEASE_URL=$(echo "${releases}" |\
  grep "browser_download.*release.yaml" |\
  cut -d '"' -f 4 |\
  sort -V | tail -n 1)

if [ -z "$RELEASE_URL" ]; then
  echo "Version $version does not exist or is not available for release.yaml."
  exit 1
fi

curl -sLO "$RELEASE_URL"
ls release.yaml
cp ./release.yaml "$where"

popd >& /dev/null

echo "release.yaml installed to ${where}release.yaml"