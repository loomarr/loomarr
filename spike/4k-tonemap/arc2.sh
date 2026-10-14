mkdir -p /work/tmp
:
for g in hdr10_4k hdr_to_sdr4k hdr_to_1080; do srcs=h2
  for s in $srcs; do for r in 1 2 3 4 5; do echo "$g,$s,$r,$(va_run $g $s $((r*9)) 2 /work/tmp/o.mp4)" >> arc2_start.csv; done; done; done
echo "mix,n,graph,stream,real_s,user_s,sys_s" > arc2_conc.csv
conc() { local mix=$1; shift; local i=0; for spec in "$@"; do i=$((i+1)); g=${spec%:*}; s=${spec#*:}
  ( o=$(va_run $g $s $((i*3)) 60 /work/tmp/c$i.mp4); echo "$mix,$#,$g,$i,$(echo $o | cut -d, -f2-)" >> arc2_conc.csv ) & done; wait; }
conc 1x4k hdr10_4k:h1
conc 2x4k hdr10_4k:h1 hdr10_4k:h2
conc 3x4k hdr10_4k:h1 hdr10_4k:h2 hdr10_4k:h4
conc 4x4k hdr10_4k:h1 hdr10_4k:h2 hdr10_4k:h4 hdr10_4k:h1
conc 1ch_pair hdr10_4k:h1 hdr_to_1080:h1
conc 1x4k+6x1080 hdr10_4k:h1 sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd
conc 1x4k+10x1080 hdr10_4k:h1 sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd
conc 2pairs+4x1080 hdr10_4k:h1 hdr_to_1080:h1 hdr10_4k:h2 hdr_to_1080:h2 sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd
conc 1x4ksdr+6x1080 sdr4k_hevc:s4 sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd sdr_1080:sd
grep h2 arc2_start.csv; cat arc2_conc.csv; rm -rf /work/tmp
