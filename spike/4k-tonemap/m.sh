# measurement helpers, sourced inside the container
export TIMEFORMAT='%R %U %S'
IN="-analyzeduration 0 -probesize 32768 -fpsprobesize 0"
VA="-init_hw_device vaapi=va:/dev/dri/renderD128 -filter_hw_device va -hwaccel vaapi -hwaccel_device va -hwaccel_output_format vaapi"
# t1s <ffmpeg args>: "first1s_ms real user sys" — first1s = spawn → muxer has 1 s of output media (progress out_time)
t1s() { local s=$(date +%s%N)
  { time ffmpeg -nostdin -hide_banner -loglevel error -stats_period 0.02 -progress pipe:1 "$@" 2>>/work/ff.err | awk -W interactive -v s=$s '/^out_time_us=/{split($0,a,"="); if(a[2]+0>=1000000 && !d){c="date +%s%N"; c|getline n; close(c); printf "%d ", (n-s)/1e6; d=1}} END{if(!d) printf "NA "}'; } 2>&1 | tr '\n' ' '; echo; }
AUD="-map 0:a:0 -c:a aac -ac 2 -ar 48000 -b:a 192k"
FMP4="-write_tmcd 0 -f mp4 -movflags +frag_keyframe+empty_moov+default_base_moof -frag_duration 1000000"
GOP="-g 24 -bf 0 -forced-idr 1"
HDR="-color_primaries bt2020 -color_trc smpte2084 -colorspace bt2020nc"
declare -A G
G[hdr10_4k]="scale_vaapi=format=p010|-c:v hevc_vaapi -profile:v main10 -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -sei hdr $HDR"
G[sdr4k_hevc]="scale_vaapi=format=nv12:out_color_matrix=bt709|-c:v hevc_vaapi -profile:v main -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -sei 0"
G[hdr_to_sdr4k]="tonemap_vaapi=format=nv12:t=bt709:m=bt709:p=bt709|-c:v hevc_vaapi -profile:v main -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -sei 0"
G[hdr_to_1080]="scale_vaapi=w=1920:h=1080:format=p010,tonemap_vaapi=format=nv12:t=bt709:m=bt709:p=bt709|-c:v h264_vaapi -profile:v high -rc_mode QVBR -global_quality 22 -b:v 8M -maxrate 12M -sei 0"
G[sdr_1080]="scale_vaapi=w=1920:h=1080:format=nv12|-c:v h264_vaapi -profile:v high -rc_mode QVBR -global_quality 22 -b:v 8M -maxrate 12M -sei 0"
# va_run <graph> <src> <seek> <dur> <out>
va_run() { local vf=${G[$1]%%|*} enc=${G[$1]#*|}; t1s $VA $IN -ss $3 -i $2.mkv -t $4 -map 0:v:0 -vf "$vf" $enc $GOP $AUD $FMP4 -y $5 | tr -s ' ' ','; }
OCL="-init_hw_device vaapi=va:/dev/dri/renderD128 -init_hw_device opencl=ocl@va -filter_hw_device ocl -hwaccel vaapi -hwaccel_device va -hwaccel_output_format vaapi"
VK="-init_hw_device vaapi=va:/dev/dri/renderD128 -init_hw_device vulkan=vk@va -filter_hw_device vk -hwaccel vaapi -hwaccel_device va -hwaccel_output_format vaapi"
TM709="t=bt709:m=bt709:p=bt709"
H264="-c:v h264_vaapi -profile:v high -rc_mode QVBR -global_quality 22 -b:v 8M -maxrate 12M -sei 0"
H265S="-c:v hevc_vaapi -profile:v main -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -sei 0"
# tone-map graphs: name → "hwinit|vf|enc"
declare -A T
T[vaapi_1080]="$VA|scale_vaapi=w=1920:h=1080:format=p010,tonemap_vaapi=format=nv12:$TM709|$H264"
T[ocl_hable_1080]="$OCL|scale_vaapi=w=1920:h=1080:format=p010,hwmap=derive_device=opencl,tonemap_opencl=tonemap=hable:desat=0:$TM709:format=nv12,hwmap=derive_device=vaapi:reverse=1,format=vaapi|$H264"
T[ocl_bt2390_1080]="$OCL|scale_vaapi=w=1920:h=1080:format=p010,hwmap=derive_device=opencl,tonemap_opencl=tonemap=bt2390:desat=0:$TM709:format=nv12,hwmap=derive_device=vaapi:reverse=1,format=vaapi|$H264"
T[vk_bt2390_1080]="$VK|scale_vaapi=w=1920:h=1080:format=p010,hwmap=derive_device=vulkan,libplacebo=tonemapping=bt.2390:format=nv12:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv,hwmap=derive_device=vaapi:reverse=1,format=vaapi|$H264"
T[vk_dl_bt2390_1080]="$VK|scale_vaapi=w=1920:h=1080:format=p010,hwmap=derive_device=vulkan,libplacebo=tonemapping=bt.2390:format=nv12:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv,hwdownload,format=nv12,hwupload=derive_device=vaapi|$H264"
T[vaapi_4k]="$VA|tonemap_vaapi=format=nv12:$TM709|$H265S"
T[ocl_hable_4k]="$OCL|hwmap=derive_device=opencl,tonemap_opencl=tonemap=hable:desat=0:$TM709:format=nv12,hwmap=derive_device=vaapi:reverse=1,format=vaapi|$H265S"
T[vk_bt2390_4k]="$VK|hwmap=derive_device=vulkan,libplacebo=tonemapping=bt.2390:format=nv12:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv,hwmap=derive_device=vaapi:reverse=1,format=vaapi|$H265S"
tm_run() { local s=${T[$1]}; local hw=${s%%|*}; s=${s#*|}; local vf=${s%%|*} enc=${s#*|}; t1s $hw $IN -ss $3 -i $2.mkv -t $4 -map 0:v:0 -vf "$vf" $enc $GOP $AUD $FMP4 -y $5 | tr -s ' ' ','; }
VKD="-init_hw_device vulkan=vk:0 -init_hw_device vaapi=va:/dev/dri/renderD128 -filter_hw_device va -hwaccel vulkan -hwaccel_device vk -hwaccel_output_format vulkan"
PL="tonemapping=bt.2390:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv"
T[vkdec_dl_1080]="$VKD|libplacebo=w=1920:h=1080:format=nv12:$PL,hwdownload,format=nv12,hwupload|$H264"
T[vkdec_map_1080]="$VKD|libplacebo=w=1920:h=1080:format=nv12:$PL,hwmap=derive_device=vaapi,format=vaapi|$H264"
T[va_dl_pl_1080]="$VA|scale_vaapi=w=1920:h=1080:format=p010,hwdownload,format=p010le,libplacebo=format=nv12:$PL,hwupload|$H264"
H265H="-c:v hevc_vaapi -profile:v main10 -rc_mode QVBR -global_quality 22 -b:v 16M -maxrate 24M -sei hdr $HDR"
UP="scale_vaapi=w=3840:h=2160:force_original_aspect_ratio=decrease:force_divisible_by=2:format=p010"
PQ="colorspace=bt2020nc:color_primaries=bt2020:color_trc=smpte2084:range=tv"
# SDR → HDR10 (PQ, SDR white at 203 nits per BT.2408) candidates
T[s2h_scale_vaapi]="$VA|$UP:out_color_matrix=bt2020nc:out_color_primaries=bt2020:out_color_transfer=smpte2084,pad_vaapi=w=3840:h=2160|$H265H"
T[s2h_tonemap_vaapi]="$VA|$UP,tonemap_vaapi=format=p010:t=smpte2084:p=bt2020:m=bt2020nc:display=0.708 0.292|0.170 0.797|0.131 0.046|0.3127 0.3290|1000 0.005:light=1000 400,pad_vaapi=w=3840:h=2160|$H265H"
T[s2h_pl_hop]="$VA|scale_vaapi=format=nv12,hwdownload,format=nv12,libplacebo=w=3840:h=2160:force_original_aspect_ratio=decrease:pad_crop_ratio=0:format=p010le:$PQ,hwupload|$H265H"
T[s2h_pl_hop_itm]="$VA|scale_vaapi=format=nv12,hwdownload,format=nv12,libplacebo=w=3840:h=2160:force_original_aspect_ratio=decrease:pad_crop_ratio=0:inverse_tonemapping=1:format=p010le:$PQ,hwupload|$H265H"
