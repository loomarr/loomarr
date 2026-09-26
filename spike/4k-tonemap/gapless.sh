# Gapless boundary test: SDR-converted → HDR10 → SDR-converted, each item its own ffmpeg process (production shape),
# 240 frames each (10.01 s at 24000/1001). Tags in-graph via setparams; encoder flags identical for every item.
N=240; NS=$((N*1001*2))   # audio samples at 48 kHz = N*1001/24000*48000
ENCH="-c:v hevc_vaapi -profile:v main10 -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -g 24 -bf 0 -forced-idr 1"
TAG="setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc:range=tv:chroma_location=left"
PAD="pad_vaapi=w=3840:h=2160"
# SDR → PQ: libplacebo at source size on the CPU hop (BT.2408, 203-nit white), then GPU upscale + pad to 4K
S2H="scale_vaapi=format=nv12,hwdownload,format=nv12,libplacebo=format=p010le:$PQ,hwupload,$UP,$PAD,$TAG"
HDRP="$UP,$PAD,$TAG"
A="-map 0:a:0 -af aresample=48000,atrim=end_sample=$NS -c:a aac -ac 2 -b:a 192k"
mkdir -p /work/gl && cd /work/gl && rm -f *
item() { local i=$1 src=$2 vf=$3; : > /work/ff.err
  echo "item$i,$src,$(t1s $VA $IN -ss 30 -i /work/$src.mkv -frames:v $N -map 0:v:0 -vf "$vf" $ENCH $A $FMP4 -y item$i.mp4 | tr -s ' ' ',')"; grep -v PPS /work/ff.err | head -2 | cut -c1-200
  ffmpeg -nostdin -v error -i item$i.mp4 -map 0:v -c copy -bsf:v hevc_mp4toannexb,filter_units=pass_types=32-34 -frames:v 1 -f hevc ps$i.bin
  ffmpeg -nostdin -v error -i item$i.mp4 -map 0:v -c copy -bsf:v hevc_mp4toannexb,filter_units=pass_types=39 -frames:v 1 -f hevc sei$i.bin; }
item 1 sd "$S2H"; item 2 h1 "$HDRP"; item 3 la "$S2H"
md5sum ps*.bin sei*.bin; ls -l sei*.bin | awk '{print $5, $9}'
for i in 1 2 3; do ffprobe -v error -select_streams v:0 -show_entries stream=profile,pix_fmt,width,height,color_transfer:stream_side_data=side_data_type,max_luminance,max_content -of compact item$i.mp4 | cut -c1-260; done
printf "file item1.mp4\nfile item2.mp4\nfile item3.mp4\n" > list.txt
ffmpeg -nostdin -v error -f concat -i list.txt -c copy -f hls -hls_segment_type fmp4 -hls_time 1 -hls_list_size 0 -hls_playlist_type vod -hls_fmp4_init_filename init.mp4 -hls_segment_filename 'seg%03d.m4s' index.m3u8
ls | wc -l; head -8 index.m3u8
# timestamps on the joined stream: off-grid video / audio packet deltas
ffprobe -v error -select_streams v:0 -show_entries packet=pts -of csv=p=0 index.m3u8 | awk 'NR>1{d=$1-p; c[d]++} {p=$1} END{for(k in c) print "video_delta",k,c[k]}'
ffprobe -v error -select_streams a:0 -show_entries packet=pts,duration -of csv=p=0 index.m3u8 | awk -F, 'NR>1{d=$1-p; c[d]++} {p=$1} END{for(k in c) print "audio_delta",k,c[k]}'
