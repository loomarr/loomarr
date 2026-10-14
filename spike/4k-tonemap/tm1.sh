# G11 on the Arc. Run 1 of each graph is the first in this fresh container (cold OpenCL kernel cache).
echo "graph,src,run,first1s_ms,real_s,user_s,sys_s" > tm1_start.csv
for t in ocl_hable_1080 ocl_hable_4k vaapi_1080 vaapi_4k va_dl_pl_1080; do for s in h1 h4; do for r in 1 2 3 4 5; do
  echo "$t,$s,$r,$(tm_run $t $s $((r*9)) 2 /work/tmp/o.mp4)" >> tm1_start.csv; done; done; done
echo "graph,content_s,real_s,user_s,sys_s" > tm1_speed.csv
for t in ocl_hable_1080 ocl_hable_4k vaapi_1080 vaapi_4k va_dl_pl_1080; do echo "$t,30,$(tm_run $t h2 5 30 /work/tmp/o.mp4 | cut -d, -f2-)" >> tm1_speed.csv; done
echo "graph,n,stream,real_s,user_s,sys_s" > tm1_conc.csv
for t in vaapi_1080 ocl_hable_1080 va_dl_pl_1080; do for n in 2 4 6; do for i in $(seq 1 $n); do
  ( s=h1; [ $((i%2)) = 0 ] && s=h4; echo "$t,$n,$i,$(tm_run $t $s $((i*4)) 30 /work/tmp/c$i.mp4 | cut -d, -f2-)" >> tm1_conc.csv ) & done; wait; done; done
ls ~/.cache 2>&1 | head -3; cat tm1_start.csv tm1_speed.csv tm1_conc.csv
