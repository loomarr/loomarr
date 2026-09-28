# Phase 1d spike: channel watermark samples (throwaway, #1512)

Scripts behind [`project/SPIKE-watermark.md`](../../project/SPIKE-watermark.md). They are not production code and have no tests; delete
them once the watermark lands.

- `render_bugs.py`: deterministic bug renderer (Pillow). It draws TMDB logo silhouettes and four typographic
  templates as white-plus-alpha PNGs, with the equal-area size rule, baked opacity and an optional soft shadow.
- `burn.sh`: burns each bug onto real excerpts through the production NVENC graph (`playout.Build`, printed
  into `graphs.env`) with `overlay_cuda` under the GPU lock, then tiles the sample sheets.

Inputs (film excerpts, TMDB logos, fonts) and outputs (sheets with film frames) stay in a local work
directory and are never committed.
