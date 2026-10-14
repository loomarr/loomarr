# exact filter chain from internal/playout/testdata/pipeline/vaapi-intel__hevc-4k-hdr-dv.golden (fps adjusted to the source)
VF="scale_vaapi=w=1920:h=1080:force_original_aspect_ratio=decrease:force_divisible_by=2:format=p010,tonemap_vaapi=format=nv12:t=bt709:m=bt709:p=bt709,pad_vaapi=w=1920:h=1080,fps=24000/1001,setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left"
for s in h1 h4; do ffmpeg -nostdin -v error $VA $IN -ss 20 -i $s.mkv -map 0:v:0 -frames:v 48 -vf "$VF" -c:v h264_vaapi -profile:v high -rc_mode QVBR -global_quality 22 -b:v 8M -maxrate 12M -sei 0 -g 24 -bf 0 -y /work/tmp/g.mp4
 echo "$s golden-chain YAVG: $(ffmpeg -nostdin -i /work/tmp/g.mp4 -vf signalstats,metadata=print:key=lavfi.signalstats.YMAX -f null - 2>&1 | grep -o 'YMAX=[0-9.]*' | sort -u | tr '\n' ' ')"; done
vainfo 2>/dev/null | grep -i 'driver version'; dpkg -l | grep -iE 'intel-media-va-driver|libva2' | awk '{print $2,$3}'
