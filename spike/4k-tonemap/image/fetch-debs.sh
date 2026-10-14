set -e; cd "$(dirname "$0")"; mkdir -p debs; cd debs; B=https://github.com/intel; CR=26.35.39758.10; IGC=2.41.5; IGCB=22716
for u in $B/intel-graphics-compiler/releases/download/v$IGC/intel-igc-core-2_${IGC}+${IGCB}_amd64.deb \
         $B/intel-graphics-compiler/releases/download/v$IGC/intel-igc-opencl-2_${IGC}+${IGCB}_amd64.deb \
         $B/compute-runtime/releases/download/$CR/intel-opencl-icd_${CR}-0_amd64.deb \
         $B/compute-runtime/releases/download/$CR/libigdgmm12_22.10.0_amd64.deb; do curl -fsSLO "$u"; done; ls -la
