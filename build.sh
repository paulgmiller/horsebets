TAG=${1:-$(git rev-parse --short HEAD)}
docker build . -t paulgmiller/horesebets:$TAG
docker push paulgmiller/horesebets:$TAG
