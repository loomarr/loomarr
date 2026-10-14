cd /work; export TIMEFORMAT='%R %U %S'
A="$SAMPLE_H264_1080P"
HW="-hwaccel vaapi -hwaccel_device /dev/dri/renderD128 -hwaccel_output_format vaapi"
VF="scale_vaapi=w=1920:h=1080:force_original_aspect_ratio=decrease:force_divisible_by=2:format=nv12,pad_vaapi=w=1920:h=1080,fps=30000/1001,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left"
V="-c:v h264_vaapi -profile:v high -level 4.1 -bf 0 -g 30 -sei 0 -rc_mode QVBR -b:v 8M -maxrate 12M -global_quality 22"
AU="-af aresample=48000,aformat=channel_layouts=stereo,apad -c:a aac -b:a 160k -ar 48000 -ac 2"
echo "== CPU split, 60 s content, unpaced (real user sys)"
echo "ffmpeg video+audio -> ts: $( { time ffmpeg -nostdin -loglevel error $HW -ss 300 -i "$A" -map 0:v:0 -map 0:a:0 -t 60 -vf "$VF" $V $AU -f mpegts -y /dev/null ; } 2>&1 )"
echo "ffmpeg video only  -> ts: $( { time ffmpeg -nostdin -loglevel error $HW -ss 300 -i "$A" -map 0:v:0 -t 60 -vf "$VF" $V -f mpegts -y /dev/null ; } 2>&1 )"
echo "ffmpeg audio only  -> ts: $( { time ffmpeg -nostdin -loglevel error -ss 300 -i "$A" -map 0:a:0 -t 60 $AU -f mpegts -y /dev/null ; } 2>&1 )"
printf '[{"name":"x","file":"%s","seek":300,"dur":60}]' "$A" > /tmp/s.json
echo "spikepack total (ffmpeg+packager): $( { time ./spikepack -schedule /tmp/s.json -out /tmp/o -seg 1 -runahead 0 2>/dev/null ; } 2>&1 )"
