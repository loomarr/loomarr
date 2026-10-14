# Phase 0b spike: 4K and tone-mapping (throwaway, #1512)

Measurement scripts behind [`project/SPIKE-4k-tonemap.md`](../../project/SPIKE-4k-tonemap.md). They are not production code and have no tests; delete them once phase 2 lands.
- `run.sh gpu|cpu <script> [image]`: throwaway container of the production image (or the `image/` test variant), `--cpus 4 -m 4g`, library read-only.
- `m.sh`: helpers; `t1s` = spawn → the muxer holds 1 s of output media (ffmpeg `-progress`, 20 ms stats period).
- `arc*.sh` G10 on the Arc; `tm*.sh`, `vk*.sh`, `vppchk*.sh` G11; `s2h*.sh`, `gapless*.sh` SDR→HDR10 and boundaries;
  `ladder*.sh` CPU-only ladder; `curves.sh` the maintainer's curve sheets (output never committed); `nv.sh` NVENC + container OpenCL.
- `play/`: a local test server (static HEVC sets + a synthetic 900-entry live DVR playlist with optional EXT-X-SKIP) and an hls.js harness.
