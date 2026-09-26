#!/bin/bash
# usage: run.sh <script> [extra docker args...]; runs a throwaway container from the prod image.
S=$1; shift
IMG=$(docker inspect loomarr --format '{{.Config.Image}}')
docker run --rm -i --device /dev/dri:/dev/dri --group-add 989 --group-add 985 --cpus 4 -m 4g \
  -v $MEDIA_ROOT/tv:/media/tv:ro -v $MEDIA_ROOT/movies:/media/movies:ro \
  -v $FILLER_DIR:/filler:ro -v /tmp/spike1512:/work "$@" \
  --entrypoint /bin/bash "$IMG" -s < /tmp/spike1512/$S
