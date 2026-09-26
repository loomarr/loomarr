: > ff.err; tm_run vk_bt2390_1080 h1 20 3 /work/tmp/vk.mp4 >/dev/null; grep -v 'PPS changed' ff.err | head -8 | cut -c1-220
echo ---; : > ff.err
ffmpeg -nostdin -hide_banner -loglevel error $VK $IN -ss 20 -i h1.mkv -t 2 -map 0:v:0 -vf "scale_vaapi=w=1920:h=1080:format=p010,hwmap=derive_device=vulkan,libplacebo=tonemapping=bt.2390:format=yuv420p:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv,hwdownload,format=yuv420p" -f null - 2>&1 | grep -v 'PPS changed' | head -5 | cut -c1-220
echo ---own-vk-device-sw-in
ffmpeg -nostdin -hide_banner -loglevel error $IN -ss 20 -i h1.mkv -t 2 -map 0:v:0 -vf "libplacebo=w=1920:h=1080:tonemapping=bt.2390:format=yuv420p:colorspace=bt709:color_primaries=bt709:color_trc=bt709:range=tv" -f null - 2>&1 | grep -v 'PPS changed' | head -5 | cut -c1-220; echo rc=$?
