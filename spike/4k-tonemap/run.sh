#!/bin/bash
# MEDIA_ROOT = the library mount on the host (tv/ and movies/ below it).
# usage: run.sh <gpu|cpu> <script> [image]; throwaway container of the prod image (or a test variant), --cpus 4 -m 4g.
M=$1; SC=$2; IMG=${3:-$(docker inspect loomarr --format '{{.Config.Image}}')}
DEV=""; [ "$M" = gpu ] && DEV="--device /dev/dri:/dev/dri --group-add 989 --group-add 985"
docker run --rm -i $DEV --cpus 4 -m 4g -v $MEDIA_ROOT/movies:/media/movies:ro -v $MEDIA_ROOT/tv:/media/tv:ro \
  -v /tmp/spike1512b:/work -w /work --entrypoint /bin/bash "$IMG" -c "source /work/m.sh; source /work/$SC"
