# kubesync

Find secrets and sync them with Google Secrets Manager

## Build images

### Install ko

```sh
go install github.com/google/ko@latest
```

### Test .dockerignore file

```sh
rsync -avn . /dev/shm --exclude-from .dockerignore
```

### Setup and publish

```sh
export KO_DOCKER_REPO=ghcr.io/ahanafy/kubesync
# export KO_DOCKER_REPO=ko.local
# microk8s
# export KO_DOCKER_REPO=localhost:32000
# microk8s ctr images ls
ko build .
```

### Local build/publish

```sh
ko resolve --local -f build/deploy.yaml > release.yaml;
IMAGENAME=$(cat release.yaml | yq -N '.spec.template.spec.containers[0] | with_entries( select( .value != null ) ) .image');
docker save $IMAGENAME > tmp-image.tar;
microk8s ctr image import tmp-image.tar;
microk8s ctr images ls | grep $IMAGENAME;
rm -f tmp-image.tar;
kubectl apply -f release.yaml;
```