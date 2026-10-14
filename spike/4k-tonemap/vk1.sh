for t in vkdec_dl_1080 vkdec_map_1080 va_dl_pl_1080; do : > ff.err; echo "$t,$(tm_run $t h1 20 10 /work/tmp/$t.mp4)"; grep -vE 'PPS changed|FINISHME' ff.err | head -3 | cut -c1-200; done
