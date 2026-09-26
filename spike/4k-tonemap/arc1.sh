# Arc: G10 premium formats + 1080p baseline. 30 s content from a warm tmpfs excerpt, audio → AAC stereo.
AUD="-map 0:a:0 -c:a aac -ac 2 -ar 48000 -b:a 192k"
FMP4="-f mp4 -movflags +frag_keyframe+empty_moov+default_base_moof -frag_duration 1000000"
GOP="-g 24 -bf 0 -forced-idr 1"
HDR="-color_primaries bt2020 -color_trc smpte2084 -colorspace bt2020nc"
declare -A G
G[hdr10_4k]="scale_vaapi=format=p010|-c:v hevc_vaapi -profile:v main10 -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -sei hdr $HDR"
G[sdr4k_hevc]="scale_vaapi=format=nv12:out_color_matrix=bt709|-c:v hevc_vaapi -profile:v main -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -sei 0"
G[hdr_to_sdr4k]="tonemap_vaapi=format=nv12:t=bt709:m=bt709:p=bt709|-c:v hevc_vaapi -profile:v main -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -sei 0"
G[hdr_to_1080]="scale_vaapi=w=1920:h=1080:format=p010,tonemap_vaapi=format=nv12:t=bt709:m=bt709:p=bt709|-c:v h264_vaapi -profile:v high -rc_mode QVBR -global_quality 22 -b:v 8M -maxrate 12M -sei 0"
echo "graph,src,run,first1s_ms,real_s,user_s,sys_s" > arc1.csv
for g in hdr10_4k sdr4k_hevc hdr_to_sdr4k hdr_to_1080; do vf=${G[$g]%%|*}; enc=${G[$g]#*|}
  srcs="h1 h2"; [ $g = sdr4k_hevc ] && srcs="s4"
  for s in $srcs; do for r in 1 2; do
    o=$(t1s $VA $IN -ss 10 -i $s.mkv -t 30 -map 0:v:0 -vf "$vf" $enc $GOP $AUD $FMP4 -y out_${g}_$s.mp4)
    echo "$g,$s,$r,$(echo $o | tr ' ' ',')" >> arc1.csv; done; done; done
cat arc1.csv
for f in out_hdr10_4k_h1 out_hdr10_4k_h2 out_sdr4k_hevc_s4; do echo "== $f"; ffprobe -v error -show_entries stream=codec_name,profile,pix_fmt,color_transfer,color_primaries,color_space:stream_side_data -of compact $f.mp4 | head -3; ffprobe -v error -select_streams v:0 -read_intervals %+0.05 -show_frames -show_entries frame=side_data_list -of compact $f.mp4 | head -1 | cut -c1-300; done
