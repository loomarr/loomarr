cd /work/gl; for i in 1 2 3; do v=$(ffprobe -v error -select_streams v:0 -count_packets -show_entries stream=nb_read_packets -of csv=p=0 item$i.mp4)
  a=$(ffprobe -v error -select_streams a:0 -show_entries packet=duration -of csv=p=0 item$i.mp4 | awk '{s+=$1} END{print s}')
  e=$(ffprobe -v error -select_streams a:0 -show_entries stream=start_time,initial_padding -of csv=p=0 item$i.mp4)
  echo "hdr item$i video_frames=$v audio_samples=$a (target $((240*1001*2))) start,padding=$e"; done
# SDR 4K premium set: SD upscaled → native 4K SDR → HDR tone-mapped to 4K SDR by OpenCL (Hable) → SD upscaled
N=240; NS=$((N*1001*2)); mkdir -p /work/gls && cd /work/gls && rm -f *
ENCS="-c:v hevc_vaapi -profile:v main -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -sei 0 -g 24 -bf 0 -forced-idr 1"
TAGS="setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv:chroma_location=left"
UP8="scale_vaapi=w=3840:h=2160:force_original_aspect_ratio=decrease:force_divisible_by=2:format=nv12,pad_vaapi=w=3840:h=2160"
A="-map 0:a:0 -af aresample=48000,atrim=end_sample=$NS -c:a aac -ac 2 -b:a 192k"
it() { : > /work/ff.err; echo "sdr item$1,$2,$(t1s $3 $IN -ss 30 -i /work/$2.mkv -frames:v $N -map 0:v:0 -vf "$4" $ENCS $A $FMP4 -y item$1.mp4 | tr -s ' ' ',')"; grep -v PPS /work/ff.err | head -2 | cut -c1-160
  ffmpeg -nostdin -v error -i item$1.mp4 -map 0:v -c copy -bsf:v hevc_mp4toannexb,filter_units=pass_types=32-34 -frames:v 1 -f hevc ps$1.bin; }
it 1 sd "$VA" "$UP8,$TAGS"; it 2 s4 "$VA" "$UP8,$TAGS"
it 3 h1 "$OCL" "hwmap=derive_device=opencl,tonemap_opencl=tonemap=hable:desat=0:$TM709:format=nv12,hwmap=derive_device=vaapi:reverse=1,format=vaapi,$UP8,$TAGS"; it 4 sd "$VA" "$UP8,$TAGS"
md5sum ps*.bin; printf "file item%d.mp4\n" 1 2 3 4 > list.txt
ffmpeg -nostdin -v error -f concat -i list.txt -c copy -f hls -hls_segment_type fmp4 -hls_time 1 -hls_list_size 0 -hls_playlist_type vod -hls_fmp4_init_filename init.mp4 -hls_segment_filename 'seg%03d.m4s' index.m3u8; ls | wc -l
