# Runs on the host: 90 s stream-copy excerpts into /tmp/spike1512b (tmpfs). Operator sets H1..H4 (4K HDR films: dark horror
# HDR10, bright desert sci-fi DV P7, family comedy DV P7, 80s action HDR10), S4 (4K SDR 10-bit film), SD (1080p SDR H.264
# cartoon), LA (1080p SDR B&W film, 1484x1080), and writes them to samples.env as KEY=path lines. Titles never committed.
set -e; IMG=$(docker inspect loomarr --format '{{.Config.Image}}')
cut() { docker run --rm -v "$(dirname "$1")":/in:ro -v /tmp/spike1512b:/work --entrypoint ffmpeg $IMG -nostdin -hide_banner -loglevel error -y -ss $2 -i "/in/$(basename "$1")" -t 90 -map 0:v:0 -map 0:a:0 -c copy /work/$3.mkv; }
cut "$H1" 1200 h1; cut "$H2" 1800 h2; cut "$H3" 1500 h3; cut "$H4" 1500 h4; cut "$S4" 1500 s4; cut "$SD" 60 sd; cut "$LA" 1200 la
