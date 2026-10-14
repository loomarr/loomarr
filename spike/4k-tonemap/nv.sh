# NVENC (RTX 3080 Ti, native ffmpeg n9.0.2) + the NVIDIA container OpenCL path. Run under the shared heavy lock.
W=$(dirname "$0"); cd $W; source ./m.sh; mkdir -p tmp
CU="-init_hw_device cuda=cu:0 -filter_hw_device cu -hwaccel cuda -hwaccel_device cu -hwaccel_output_format cuda"
CUO="-init_hw_device cuda=cu:0 -init_hw_device opencl=ocl -filter_hw_device ocl -hwaccel cuda -hwaccel_device cu -hwaccel_output_format cuda"
NH="-c:v hevc_nvenc -profile:v main10 -preset p4 -rc vbr -cq 22 -b:v 16M -maxrate 24M $HDR"
NS="-c:v hevc_nvenc -profile:v main -preset p4 -rc vbr -cq 22 -b:v 16M -maxrate 24M"
N4="-c:v h264_nvenc -profile:v high -preset p4 -rc vbr -cq 22 -b:v 8M -maxrate 12M"
FIT="w=1920:h=1080:force_original_aspect_ratio=decrease:force_divisible_by=2"
declare -A N
N[hdr10_4k]="$CU|scale_cuda=format=p010le|$NH"
N[sdr4k]="$CU|scale_cuda=format=nv12|$NS"
N[sdr_1080]="$CU|scale_cuda=$FIT:format=nv12|$N4"
N[ocl_1080]="$CUO|scale_cuda=$FIT:format=p010le,hwdownload,format=p010le,hwupload,tonemap_opencl=tonemap=hable:desat=0:$TM709:r=tv:format=nv12,hwdownload,format=nv12,hwupload_cuda|$N4"
N[pl_1080]="$CU|scale_cuda=$FIT:format=p010le,hwdownload,format=p010le,libplacebo=format=nv12:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv:tonemapping=bt.2390,hwupload_cuda|$N4"
N[ocl_4k]="$CUO|scale_cuda=format=p010le,hwdownload,format=p010le,hwupload,tonemap_opencl=tonemap=hable:desat=0:$TM709:r=tv:format=nv12,hwdownload,format=nv12,hwupload_cuda|$NS"
nv_run() { local s=${N[$1]}; local hw=${s%%|*}; s=${s#*|}; local vf=${s%%|*} enc=${s#*|}; t1s $hw $IN -ss $3 -i $2.mkv -t $4 -map 0:v:0 -vf "$vf" $enc $GOP $AUD $FMP4 -y $5 | tr -s ' ' ','; }
echo "graph,src,run,first1s_ms,real_s,user_s,sys_s" > nv_start.csv
for g in hdr10_4k sdr4k ocl_1080 pl_1080 ocl_4k; do srcs="h1 h4"; [ $g = sdr4k ] && srcs=s4
  for s in $srcs; do for r in 1 2 3 4; do echo "$g,$s,$r,$(nv_run $g $s $((r*5)) 2 tmp/o.mp4)" >> nv_start.csv; done; done; done
echo "graph,content_s,real_s,user_s,sys_s" > nv_speed.csv
for g in hdr10_4k sdr4k ocl_1080 pl_1080 ocl_4k; do s=h1; [ $g = sdr4k ] && s=s4; echo "$g,30,$(nv_run $g $s 5 30 tmp/o.mp4 | cut -d, -f2-)" >> nv_speed.csv; done
ffprobe -v error -show_entries stream_side_data=side_data_type,max_luminance,max_content -select_streams v:0 -of compact tmp/o.mp4 >/dev/null
nv_run hdr10_4k h1 5 3 tmp/hdr.mp4 >/dev/null; echo "hdr10 side data: $(ffprobe -v error -select_streams v:0 -show_entries stream=color_transfer:stream_side_data=side_data_type -of compact tmp/hdr.mp4)"
echo "mix,graph,stream,real_s,user_s,sys_s" > nv_conc.csv
conc() { local mix=$1; shift; local i=0; for spec in "$@"; do i=$((i+1)); g=${spec%:*}; s=${spec#*:}
  ( echo "$mix,$g,$i,$(nv_run $g $s $((i%5)) 30 tmp/c$i.mp4 | cut -d, -f2-)" >> nv_conc.csv ) & done; wait; }
conc 3x4k hdr10_4k:h1 hdr10_4k:h4 hdr10_4k:h1
conc 1x4k+1ocl+6x1080 hdr10_4k:h1 ocl_1080:h1 sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd
conc 4xocl_1080 ocl_1080:h1 ocl_1080:h4 ocl_1080:h1 ocl_1080:h4
# the NVIDIA container path: production image, CDI device, compute+video+utility
IMG=ghcr.io/loomarr/loomarr:0.2.0-beta.7; DR="docker run --rm --device nvidia.com/gpu=all -e NVIDIA_DRIVER_CAPABILITIES=compute,video,utility -v $W:/w -w /w --entrypoint /bin/bash"
$DR $IMG -c 'ls /etc/OpenCL/vendors 2>&1 | head -1; ffmpeg -v error -init_hw_device opencl=ocl -f lavfi -i nullsrc -frames 1 -f null - 2>&1 | head -1; echo "prod image opencl rc=$?"'
echo libnvidia-opencl.so.1 > tmp/nvidia.icd
$DR -v $W/tmp/nvidia.icd:/etc/OpenCL/vendors/nvidia.icd:ro $IMG -c 'ffmpeg -v error -init_hw_device opencl=ocl -f lavfi -i nullsrc -frames 1 -f null - 2>&1 | head -1; echo "with nvidia.icd rc=$?"; sed "s/awk -v s/awk -W interactive -v s/" m.sh > /tmp/m.sh; source /tmp/m.sh; W=/w; source <(sed -n "/^CU=/,/^nv_run/p" nv.sh); for r in 1 2 3 4; do echo "container ocl_1080,$(nv_run ocl_1080 h1 $((r*5)) 2 /tmp/o.mp4)"; done; echo "container ocl_1080 speed,$(nv_run ocl_1080 h1 5 30 /tmp/o.mp4)"'
cat nv_start.csv nv_speed.csv nv_conc.csv
