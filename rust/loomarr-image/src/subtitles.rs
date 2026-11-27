//! `decode-subtitles --protocol 1`: an image subtitle track in, timed full-canvas stills out.
//!
//! A codec decoder reports each display state to a [`Timeline`] as a start time and a render step
//! that draws it onto a full straight-alpha RGBA canvas. The timeline is codec-neutral: it keeps
//! states in order, folds repeated clears, charges every draw, clear and encode to the work
//! ceilings before doing it, enforces the state and output ceilings, and writes one PNG per still
//! into staging. PGS (`hdmv_pgs_subtitle`, a `.sup`) and VOBSUB (`dvd_subtitle`, an `.idx`/`.sub`
//! pair, in [`vobsub`]) decode behind the same timeline.

use std::collections::HashMap;
use std::fs;
use std::io::Read;
use std::path::{Path, PathBuf};

mod vobsub;

pub use vobsub::VOBSUB_CODEC;

use image::ImageEncoder;
use image::codecs::png::{CompressionType, FilterType, PngEncoder};
use serde::{Deserialize, Serialize};

use crate::{PROTOCOL, WorkerError, hex_digest, read_control, write_complete, write_error};

/// The hard ceilings of design `images.md` *Operations beyond renditions*.
#[derive(Clone, Copy, Debug)]
pub(crate) struct Limits {
    pub max_input_bytes: u64,
    /// A VOBSUB `.idx`: about 4x the 2.2 MB index of a track at the state ceiling, 50,000
    /// 44-byte `timestamp:` lines.
    pub max_index_bytes: u64,
    pub max_width: u32,
    pub max_height: u32,
    pub max_objects_per_state: usize,
    pub max_states: usize,
    pub max_output_bytes: u64,
    /// Object pixels decoded across every display set: each placement charges its object's area
    /// plus its coded length, before it is drawn.
    pub max_drawn_pixels: u64,
    /// Canvas pixels encoded or cleared across every display set, charged before each encode and
    /// each clear.
    pub max_canvas_pixels: u64,
}

pub(crate) const LIMITS: Limits = Limits {
    max_input_bytes: 256 << 20,
    max_index_bytes: 8 << 20,
    max_width: 3840,
    max_height: 2160,
    max_objects_per_state: 64,
    max_states: 50_000,
    max_output_bytes: 512 << 20,
    max_drawn_pixels: DRAWN_PIXELS_CEILING,
    max_canvas_pixels: CANVAS_PIXELS_CEILING,
};

// About 4x the heavier track `heavy_realistic_tracks_fit_the_work_ceilings` measures: 3,000
// display sets of two 3840x400 objects on a 3840x2160 canvas, each cleared, draw 9.45e9 object
// pixels and encode or clear 4.98e10 canvas pixels in about 26 s of a release build on a shared
// 24-core machine.
const DRAWN_PIXELS_CEILING: u64 = 38_000_000_000;
const CANVAS_PIXELS_CEILING: u64 = 200_000_000_000;

pub const PGS_CODEC: &str = "hdmv_pgs_subtitle";

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct DecodeRequest {
    protocol: u32,
    request_id: String,
    source: TrackRequest,
    staging_dir: PathBuf,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct TrackRequest {
    codec: String,
    /// The `.sup` of a PGS track, the `.sub` of a VOBSUB pair.
    path: PathBuf,
    expected_sha256: String,
    /// The `.idx` of a VOBSUB pair: required for `dvd_subtitle`, refused for PGS.
    index: Option<FileRequest>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct FileRequest {
    path: PathBuf,
    expected_sha256: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct DecodeResult {
    protocol: u32,
    status: &'static str,
    request_id: String,
    source: TrackResult,
    width: u32,
    height: u32,
    states: Vec<StateResult>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct TrackResult {
    codec: &'static str,
    sha256: String,
    bytes: u64,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct StateResult {
    start_ms: i64,
    end_ms: i64,
    still: Option<StillResult>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct StillResult {
    relative_path: String,
    bytes: u64,
    sha256: String,
}

/// One full canvas of straight-alpha RGBA, `width * height * 4` bytes.
pub(crate) struct Canvas {
    pub width: u32,
    pub height: u32,
    pub pixels: Vec<u8>,
    /// Whether any pixel was drawn since the canvas was last cleared.
    pub dirty: bool,
}

impl Canvas {
    pub(crate) fn new(width: u32, height: u32) -> Self {
        Self {
            width,
            height,
            pixels: vec![0; width as usize * height as usize * 4],
            dirty: false,
        }
    }

    fn area(&self) -> u64 {
        u64::from(self.width) * u64::from(self.height)
    }
}

/// The work a track has cost so far, counted across every display set and charged before the work
/// is done, so a track that would exceed a ceiling is refused instead of decoded.
#[derive(Clone, Copy, Debug, Default, PartialEq, Eq)]
pub(crate) struct Work {
    pub drawn_pixels: u64,
    pub canvas_pixels: u64,
}

pub(crate) struct Budget {
    limits: Limits,
    work: Work,
}

impl Budget {
    pub(crate) fn draw(&mut self, pixels: u64) -> Result<(), WorkerError> {
        self.work.drawn_pixels = self.work.drawn_pixels.saturating_add(pixels);
        if self.work.drawn_pixels > self.limits.max_drawn_pixels {
            return Err(WorkerError::new(
                "limit_exceeded",
                "drawn object pixels exceed the work ceiling",
            ));
        }
        Ok(())
    }

    fn canvas(&mut self, pixels: u64) -> Result<(), WorkerError> {
        self.work.canvas_pixels = self.work.canvas_pixels.saturating_add(pixels);
        if self.work.canvas_pixels > self.limits.max_canvas_pixels {
            return Err(WorkerError::new(
                "limit_exceeded",
                "encoded and cleared canvas pixels exceed the work ceiling",
            ));
        }
        Ok(())
    }
}

/// Where a still's encoded PNG goes: staging for a request, memory for the self-test.
pub(crate) trait StillSink {
    fn write(&mut self, index: usize, png: &[u8]) -> Result<String, WorkerError>;
}

struct Staging(PathBuf);

impl StillSink for Staging {
    fn write(&mut self, index: usize, png: &[u8]) -> Result<String, WorkerError> {
        let name = format!("s{index:05}.png");
        write_complete(&self.0, &name, png)?;
        Ok(name)
    }
}

/// The codec-neutral display timeline. Each state lasts until the next one starts; the last has
/// no successor and holds until the programme ends, so its end equals its start. A state at the
/// same instant as the one before it replaces it, so the canvas keeps the latest state's pixels and
/// a PNG is encoded only once a later instant makes that state's time final.
pub(crate) struct Timeline<'a, S: StillSink> {
    limits: Limits,
    sink: &'a mut S,
    states: Vec<StateResult>,
    /// The latest state, not yet emitted: its start and whether the canvas shows a still.
    pending: Option<(i64, bool)>,
    /// The latest start pushed. A replacement can remove a state, so order is checked against
    /// this rather than against what remains.
    latest_ms: Option<i64>,
    budget: Budget,
    output_bytes: u64,
}

impl<'a, S: StillSink> Timeline<'a, S> {
    pub(crate) fn new(limits: Limits, sink: &'a mut S) -> Self {
        Self {
            limits,
            sink,
            states: Vec::new(),
            pending: None,
            latest_ms: None,
            budget: Budget {
                limits,
                work: Work::default(),
            },
            output_bytes: 0,
        }
    }

    /// Adds the state that starts at `start_ms`. `render` draws it onto the cleared canvas,
    /// charging what it draws to the budget, and reports whether anything shows.
    pub(crate) fn push(
        &mut self,
        start_ms: i64,
        canvas: &mut Canvas,
        render: impl FnOnce(&mut Canvas, &mut Budget) -> Result<bool, WorkerError>,
    ) -> Result<(), WorkerError> {
        if self.latest_ms.is_some_and(|latest| start_ms < latest) {
            return Err(WorkerError::new(
                "corrupt_input",
                "display states go back in time",
            ));
        }
        self.latest_ms = Some(start_ms);
        match self.pending {
            Some((pending_start, _)) if start_ms > pending_start => self.flush(canvas)?,
            Some(_) => self.pending = None,
            None => {}
        }
        if canvas.dirty {
            self.budget.canvas(canvas.area())?;
            canvas.pixels.fill(0);
            canvas.dirty = false;
        }
        let drawn = render(canvas, &mut self.budget)?;
        canvas.dirty = drawn;
        let previous_clear = self.states.last().is_none_or(|state| state.still.is_none());
        if drawn || !previous_clear {
            self.pending = Some((start_ms, drawn));
        }
        Ok(())
    }

    fn flush(&mut self, canvas: &Canvas) -> Result<(), WorkerError> {
        let Some((start_ms, drawn)) = self.pending.take() else {
            return Ok(());
        };
        if self.states.len() >= self.limits.max_states {
            return Err(WorkerError::new(
                "limit_exceeded",
                "display states exceed the ceiling",
            ));
        }
        if let Some(last) = self.states.last_mut() {
            last.end_ms = start_ms;
        }
        let still = if drawn {
            self.budget.canvas(canvas.area())?;
            let png = encode_png(canvas)?;
            self.output_bytes = self.output_bytes.saturating_add(png.len() as u64);
            if self.output_bytes > self.limits.max_output_bytes {
                return Err(WorkerError::new(
                    "limit_exceeded",
                    "stills exceed the output ceiling",
                ));
            }
            let relative_path = self.sink.write(self.states.len(), &png)?;
            Some(StillResult {
                relative_path,
                bytes: png.len() as u64,
                sha256: hex_digest(&png),
            })
        } else {
            None
        };
        self.states.push(StateResult {
            start_ms,
            end_ms: start_ms,
            still,
        });
        Ok(())
    }

    fn finish(mut self, canvas: &Canvas) -> Result<(Vec<StateResult>, Work), WorkerError> {
        self.flush(canvas)?;
        Ok((self.states, self.budget.work))
    }
}

/// A decoded track: the canvas size, its states, and the work they cost.
struct Decoded {
    width: u32,
    height: u32,
    states: Vec<StateResult>,
    work: Work,
}

fn encode_png(canvas: &Canvas) -> Result<Vec<u8>, WorkerError> {
    let mut out = Vec::new();
    PngEncoder::new_with_quality(&mut out, CompressionType::Fast, FilterType::Adaptive)
        .write_image(
            &canvas.pixels,
            canvas.width,
            canvas.height,
            image::ExtendedColorType::Rgba8,
        )
        .map_err(|err| WorkerError::new("encode_failed", format!("encode PNG: {err}")))?;
    Ok(out)
}

pub fn run_decode_subtitles(
    input: impl Read,
    output: impl std::io::Write,
) -> Result<(), WorkerError> {
    let control = read_control(input)?;
    let request: DecodeRequest = serde_json::from_slice(&control)
        .map_err(|err| WorkerError::new("invalid_request", format!("request JSON: {err}")))?;
    let request_id = request.request_id.clone();
    match decode(request, LIMITS) {
        Ok(result) => serde_json::to_writer(output, &result)
            .map_err(|err| WorkerError::new("internal", format!("result JSON: {err}"))),
        Err(err) => write_error(output, &request_id, err),
    }
}

fn decode(request: DecodeRequest, limits: Limits) -> Result<DecodeResult, WorkerError> {
    if request.protocol != PROTOCOL {
        return Err(WorkerError::new("invalid_request", "protocol mismatch"));
    }
    let codec = match request.source.codec.as_str() {
        PGS_CODEC => PGS_CODEC,
        VOBSUB_CODEC => VOBSUB_CODEC,
        _ => {
            return Err(WorkerError::new(
                "unsupported_input",
                "subtitle codec is not decodable",
            ));
        }
    };
    match (codec, &request.source.index) {
        (VOBSUB_CODEC, None) => {
            return Err(WorkerError::new(
                "invalid_request",
                "a dvd_subtitle source needs its index",
            ));
        }
        (PGS_CODEC, Some(_)) => {
            return Err(WorkerError::new(
                "invalid_request",
                "a hdmv_pgs_subtitle source has no index",
            ));
        }
        _ => {}
    }
    let source = read_source(
        &request.source.path,
        &request.source.expected_sha256,
        limits.max_input_bytes,
        "track exceeds the input ceiling",
    )?;
    let index = match &request.source.index {
        Some(index) => Some(read_source(
            &index.path,
            &index.expected_sha256,
            limits.max_index_bytes,
            "index exceeds the index ceiling",
        )?),
        None => None,
    };
    let mut staging = Staging(request.staging_dir);
    let Decoded {
        width,
        height,
        states,
        ..
    } = match &index {
        Some(index) => vobsub::decode_vobsub(index, &source, limits, &mut staging)?,
        None => decode_pgs(&source, limits, &mut staging)?,
    };
    Ok(DecodeResult {
        protocol: PROTOCOL,
        status: "ok",
        request_id: request.request_id,
        source: TrackResult {
            codec,
            sha256: request.source.expected_sha256,
            bytes: source.len() as u64,
        },
        width,
        height,
        states,
    })
}

/// Reads one regular file of at most `max` bytes and checks it against its expected SHA-256.
fn read_source(
    path: &Path,
    expected_sha256: &str,
    max: u64,
    over_ceiling: &'static str,
) -> Result<Vec<u8>, WorkerError> {
    let meta = fs::symlink_metadata(path)
        .map_err(|err| WorkerError::new("io_failed", format!("stat source: {err}")))?;
    if !meta.is_file() {
        return Err(WorkerError::new(
            "invalid_request",
            "source is not a regular file",
        ));
    }
    if meta.len() > max {
        return Err(WorkerError::new("limit_exceeded", over_ceiling));
    }
    let mut source = Vec::new();
    fs::File::open(path)
        .and_then(|file| file.take(max + 1).read_to_end(&mut source))
        .map_err(|err| WorkerError::new("io_failed", format!("read source: {err}")))?;
    if source.len() as u64 > max {
        return Err(WorkerError::new("limit_exceeded", over_ceiling));
    }
    if hex_digest(&source) != expected_sha256 {
        return Err(WorkerError::new(
            "source_changed",
            "source SHA-256 does not match request",
        ));
    }
    Ok(source)
}

// PGS segment types.
const PALETTE: u8 = 0x14;
const OBJECT: u8 = 0x15;
const COMPOSITION: u8 = 0x16;
const WINDOW: u8 = 0x17;
const END: u8 = 0x80;
/// The composition state that discards every object and palette before it.
const EPOCH_START: u8 = 0x80;

struct Placement {
    object: u16,
    x: u32,
    y: u32,
    /// The object's source rectangle: `(x0, y0, x1, y1)`, exclusive on the far edges.
    crop: Option<(u32, u32, u32, u32)>,
}

struct Object {
    width: u32,
    height: u32,
    data: Vec<u8>,
}

fn corrupt(message: &'static str) -> WorkerError {
    WorkerError::new("corrupt_input", message)
}

fn be16(bytes: &[u8]) -> u32 {
    u32::from(u16::from_be_bytes([bytes[0], bytes[1]]))
}

/// Decodes a `.sup` stream (ffmpeg's `-c:s copy -f sup`) into the canvas size and its states.
/// A display set's composition takes effect at its presentation time (90 kHz, rounded to the
/// nearest millisecond) when its END segment arrives.
fn decode_pgs<S: StillSink>(
    mut bytes: &[u8],
    limits: Limits,
    sink: &mut S,
) -> Result<Decoded, WorkerError> {
    let mut palettes: HashMap<u8, [[u8; 4]; 256]> = HashMap::new();
    let mut objects: HashMap<u16, Object> = HashMap::new();
    let mut canvas: Option<Canvas> = None;
    let mut timeline = Timeline::new(limits, sink);
    let mut start_ms = 0_i64;
    let mut palette_id = 0_u8;
    let mut placed: Vec<Placement> = Vec::new();
    let mut pending = false;
    while !bytes.is_empty() {
        if bytes.len() < 13 || bytes[0] != b'P' || bytes[1] != b'G' {
            return Err(corrupt("bad segment header"));
        }
        let pts = u32::from_be_bytes([bytes[2], bytes[3], bytes[4], bytes[5]]);
        let kind = bytes[10];
        let size = be16(&bytes[11..13]) as usize;
        if bytes.len() < 13 + size {
            return Err(corrupt("truncated segment"));
        }
        let seg = &bytes[13..13 + size];
        bytes = &bytes[13 + size..];
        match kind {
            COMPOSITION => {
                if seg.len() < 11 {
                    return Err(corrupt("short composition"));
                }
                let (width, height) = (be16(&seg[0..2]), be16(&seg[2..4]));
                match &canvas {
                    Some(c) if (c.width, c.height) != (width, height) => {
                        return Err(corrupt("a composition changes the canvas size"));
                    }
                    Some(_) => {}
                    None => {
                        if width == 0 || height == 0 {
                            return Err(corrupt("empty canvas"));
                        }
                        if width > limits.max_width || height > limits.max_height {
                            return Err(WorkerError::new(
                                "limit_exceeded",
                                "canvas exceeds the ceiling",
                            ));
                        }
                        canvas = Some(Canvas::new(width, height));
                    }
                }
                if seg[7] & EPOCH_START != 0 {
                    palettes.clear();
                    objects.clear();
                }
                start_ms = ((u64::from(pts) + 45) / 90) as i64;
                palette_id = seg[9];
                let count = usize::from(seg[10]);
                if count > limits.max_objects_per_state {
                    return Err(WorkerError::new(
                        "limit_exceeded",
                        "objects per state exceed the ceiling",
                    ));
                }
                placed.clear();
                pending = true;
                let mut rest = &seg[11..];
                for _ in 0..count {
                    if rest.len() < 8 {
                        return Err(corrupt("short composition object"));
                    }
                    let cropped = rest[3] & 0x80 != 0;
                    let mut placement = Placement {
                        object: be16(&rest[0..2]) as u16,
                        x: be16(&rest[4..6]),
                        y: be16(&rest[6..8]),
                        crop: None,
                    };
                    rest = &rest[8..];
                    if cropped {
                        if rest.len() < 8 {
                            return Err(corrupt("short crop"));
                        }
                        let (cx, cy) = (be16(&rest[0..2]), be16(&rest[2..4]));
                        let (cw, ch) = (be16(&rest[4..6]), be16(&rest[6..8]));
                        placement.crop = Some((cx, cy, cx + cw, cy + ch));
                        rest = &rest[8..];
                    }
                    placed.push(placement);
                }
            }
            PALETTE => {
                if seg.len() < 2 {
                    return Err(corrupt("short palette"));
                }
                let palette = palettes.entry(seg[0]).or_insert([[0; 4]; 256]);
                for [index, y, cr, cb, alpha] in seg[2..].as_chunks::<5>().0 {
                    palette[usize::from(*index)] = ycbcr_to_rgba(*y, *cr, *cb, *alpha);
                }
            }
            OBJECT => {
                if seg.len() < 4 {
                    return Err(corrupt("short object"));
                }
                let id = be16(&seg[0..2]) as u16;
                if seg[3] & 0x80 != 0 {
                    // The first fragment carries the object's size.
                    if seg.len() < 11 {
                        return Err(corrupt("short first object fragment"));
                    }
                    let (width, height) = (be16(&seg[7..9]), be16(&seg[9..11]));
                    if width > limits.max_width || height > limits.max_height {
                        return Err(WorkerError::new(
                            "limit_exceeded",
                            "object exceeds the canvas ceiling",
                        ));
                    }
                    objects.insert(
                        id,
                        Object {
                            width,
                            height,
                            data: seg[11..].to_vec(),
                        },
                    );
                } else if let Some(object) = objects.get_mut(&id) {
                    object.data.extend_from_slice(&seg[4..]);
                }
            }
            WINDOW => {
                // Windows bound where objects may draw; the composition's placements already say where.
            }
            END => {
                if !pending {
                    continue;
                }
                pending = false;
                let Some(canvas) = canvas.as_mut() else {
                    continue;
                };
                let palette = palettes.get(&palette_id);
                timeline.push(start_ms, canvas, |canvas, budget| {
                    render(canvas, budget, &placed, &objects, palette)
                })?;
            }
            _ => {}
        }
    }
    let Some(canvas) = canvas else {
        return Err(corrupt("track has no compositions"));
    };
    let (states, work) = timeline.finish(&canvas)?;
    Ok(Decoded {
        width: canvas.width,
        height: canvas.height,
        states,
        work,
    })
}

/// Draws one composition onto the cleared canvas, charging each placement before it decodes;
/// false when nothing shows.
fn render(
    canvas: &mut Canvas,
    budget: &mut Budget,
    placed: &[Placement],
    objects: &HashMap<u16, Object>,
    palette: Option<&[[u8; 4]; 256]>,
) -> Result<bool, WorkerError> {
    let Some(palette) = palette else {
        return Ok(false);
    };
    let mut drawn = false;
    for placement in placed {
        // An acquisition point can name an object a later set redefines; skip it.
        let Some(object) = objects.get(&placement.object) else {
            continue;
        };
        budget
            .draw(u64::from(object.width) * u64::from(object.height) + object.data.len() as u64)?;
        let (x0, y0, x1, y1) = match placement.crop {
            Some((cx0, cy0, cx1, cy1)) => (
                cx0.min(object.width),
                cy0.min(object.height),
                cx1.min(object.width),
                cy1.min(object.height),
            ),
            None => (0, 0, object.width, object.height),
        };
        let mut draw = |row: u32, from: u32, to: u32, index: u8| {
            let colour = palette[usize::from(index)];
            if colour[3] == 0 || row < y0 || row >= y1 {
                return;
            }
            let canvas_y = u64::from(placement.y) + u64::from(row - y0);
            if canvas_y >= u64::from(canvas.height) {
                return;
            }
            let (from, to) = (from.max(x0), to.min(x1));
            if from >= to {
                return;
            }
            // The run's span on the canvas row, clipped to the canvas's right edge.
            let left = u64::from(placement.x) + u64::from(from - x0);
            let right = (u64::from(placement.x) + u64::from(to - x0)).min(u64::from(canvas.width));
            if left >= right {
                return;
            }
            let row_at = canvas_y * u64::from(canvas.width);
            let span = ((row_at + left) * 4) as usize..((row_at + right) * 4) as usize;
            canvas.pixels[span].as_chunks_mut::<4>().0.fill(colour);
            drawn = true;
        };
        decode_rle(&object.data, object.width, object.height, &mut draw)?;
    }
    Ok(drawn)
}

/// Expands an object's run-length coded rows, reporting each run as `(row, x0, x1, index)` with
/// `x1` clipped to the object's width.
fn decode_rle(
    data: &[u8],
    width: u32,
    height: u32,
    draw: &mut impl FnMut(u32, u32, u32, u8),
) -> Result<(), WorkerError> {
    let (mut x, mut y) = (0_u32, 0_u32);
    let mut put = |x: &mut u32, y: u32, run: u32, index: u8| {
        let end = x.saturating_add(run).min(width);
        if end > *x {
            draw(y, *x, end, index);
            *x = end;
        }
    };
    let mut i = 0;
    while i < data.len() && y < height {
        let byte = data[i];
        i += 1;
        if byte != 0 {
            put(&mut x, y, 1, byte);
            continue;
        }
        let Some(&flag) = data.get(i) else {
            break;
        };
        i += 1;
        if flag == 0 {
            (x, y) = (0, y + 1);
            continue;
        }
        let mut run = u32::from(flag & 0x3f);
        if flag & 0x40 != 0 {
            let Some(&low) = data.get(i) else {
                return Err(corrupt("truncated run"));
            };
            run = run << 8 | u32::from(low);
            i += 1;
        }
        let mut index = 0;
        if flag & 0x80 != 0 {
            let Some(&colour) = data.get(i) else {
                return Err(corrupt("truncated run colour"));
            };
            index = colour;
            i += 1;
        }
        put(&mut x, y, run, index);
    }
    Ok(())
}

/// Converts a palette entry (BT.709 limited range, as Blu-ray's HD subtitles are) to straight-alpha
/// RGBA. The burn-in converts it back with the same matrix, so a colour round-trips.
pub(crate) fn ycbcr_to_rgba(y: u8, cr: u8, cb: u8, alpha: u8) -> [u8; 4] {
    let y = (f64::from(y) - 16.0) * 255.0 / 219.0;
    let cr = (f64::from(cr) - 128.0) * 255.0 / 224.0;
    let cb = (f64::from(cb) - 128.0) * 255.0 / 224.0;
    let clamp = |v: f64| v.clamp(0.0, 255.0).round() as u8;
    [
        clamp(y + 1.5748 * cr),
        clamp(y - 0.1873 * cb - 0.4681 * cr),
        clamp(y + 1.8556 * cb),
        alpha,
    ]
}

/// The ceilings the fuzz entries decode under, small enough to keep each input fast.
const FUZZ_LIMITS: Limits = Limits {
    max_width: 256,
    max_height: 256,
    max_states: 64,
    max_output_bytes: 4 << 20,
    max_drawn_pixels: 1 << 24,
    max_canvas_pixels: 1 << 23,
    ..LIMITS
};

/// The fuzz entry: the PGS decoder over arbitrary bytes, stills kept in memory.
#[doc(hidden)]
pub fn fuzz_decode_pgs(bytes: &[u8]) -> Result<usize, String> {
    let decoded = decode_pgs(bytes, FUZZ_LIMITS, &mut Memory::default())
        .map_err(|err| err.code.to_owned())?;
    fuzz_checked(decoded, FUZZ_LIMITS)
}

/// The fuzz entry: the VOBSUB decoder over an arbitrary `.idx` and `.sub`, stills kept in memory.
#[doc(hidden)]
pub fn fuzz_decode_vobsub(idx: &[u8], sub: &[u8]) -> Result<usize, String> {
    let decoded = vobsub::decode_vobsub(idx, sub, FUZZ_LIMITS, &mut Memory::default())
        .map_err(|err| err.code.to_owned())?;
    fuzz_checked(decoded, FUZZ_LIMITS)
}

/// The VOBSUB self-test probe's `.idx` and `.sub`, a valid pair for the fuzzer to mutate.
#[doc(hidden)]
pub fn fuzz_vobsub_seed() -> (Vec<u8>, Vec<u8>) {
    let mut pair = vobsub::test_support::Pair::new((8, 4), "");
    pair.spu(
        1000,
        &vobsub::test_support::Unit::two_rows(2, 1).bytes(),
        2048,
    );
    (pair.idx.into_bytes(), pair.sub)
}

fn fuzz_checked(decoded: Decoded, limits: Limits) -> Result<usize, String> {
    // A decoded track never costs more than the ceilings it was charged against.
    assert!(
        decoded.work.drawn_pixels <= limits.max_drawn_pixels
            && decoded.work.canvas_pixels <= limits.max_canvas_pixels,
        "work {:?} passed its ceilings",
        decoded.work
    );
    Ok(decoded.states.len())
}

#[derive(Default)]
pub(crate) struct Memory(pub Vec<Vec<u8>>);

impl StillSink for Memory {
    fn write(&mut self, index: usize, png: &[u8]) -> Result<String, WorkerError> {
        self.0.push(png.to_vec());
        Ok(format!("s{index:05}.png"))
    }
}

/// The PGS probe and the VOBSUB probe.
pub(crate) fn self_test() -> bool {
    pgs_self_test() && vobsub::self_test()
}

/// A 4x2 PGS probe: one still at 1 s, cleared at 4 s, on an 8x4 canvas.
fn pgs_self_test() -> bool {
    let mut w = test_support::Sup::default();
    w.show(
        90_000,
        EPOCH_START,
        2,
        1,
        &test_support::TWO_ROWS,
        4,
        2,
        (8, 4),
    );
    w.clear(360_000, (8, 4));
    let mut memory = Memory::default();
    let Ok(Decoded {
        width: 8,
        height: 4,
        states,
        ..
    }) = decode_pgs(&w.0, LIMITS, &mut memory)
    else {
        return false;
    };
    let timing: Vec<_> = states
        .iter()
        .map(|s| (s.start_ms, s.end_ms, s.still.is_some()))
        .collect();
    let Some(png) = memory.0.first() else {
        return false;
    };
    let Ok(decoded) = image::load_from_memory_with_format(png, image::ImageFormat::Png) else {
        return false;
    };
    let rgba = decoded.to_rgba8();
    timing == [(1000, 4000, true), (4000, 4000, false)]
        && rgba.dimensions() == (8, 4)
        && rgba.get_pixel(4, 1).0 == [255, 255, 255, 255]
        && rgba.get_pixel(2, 2).0 == [0, 0, 0, 128]
        && rgba.get_pixel(2, 1).0 == [0, 0, 0, 0]
}

/// A `.sup` writer laid out the way a Blu-ray authoring tool writes one; the self-test probe and
/// the tests share it.
pub(crate) mod test_support {
    use super::*;

    /// A 4x2 object: row 0 is two transparent then two white (a short zero run, then literal
    /// pixels); row 1 is a coded run of four half-black.
    pub(crate) const TWO_ROWS: [u8; 11] = [0, 0x02, 1, 1, 0, 0, 0, 0x84, 2, 0, 0];

    #[derive(Default)]
    pub(crate) struct Sup(pub Vec<u8>);

    impl Sup {
        pub(crate) fn seg(&mut self, pts: u32, kind: u8, data: &[u8]) {
            self.0.extend_from_slice(b"PG");
            self.0.extend_from_slice(&pts.to_be_bytes());
            self.0.extend_from_slice(&[0, 0, 0, 0, kind]);
            self.0.extend_from_slice(&(data.len() as u16).to_be_bytes());
            self.0.extend_from_slice(data);
        }

        /// A display set: palette 0 (index 1 opaque white, 2 half-transparent black), one object
        /// placed at x,y, its rows run-length coded.
        #[allow(clippy::too_many_arguments)]
        pub(crate) fn show(
            &mut self,
            pts: u32,
            state: u8,
            x: u16,
            y: u16,
            rle: &[u8],
            ow: u16,
            oh: u16,
            (cw, ch): (u16, u16),
        ) {
            let mut pcs = Vec::new();
            pcs.extend_from_slice(&cw.to_be_bytes());
            pcs.extend_from_slice(&ch.to_be_bytes());
            pcs.extend_from_slice(&[0x10, 0, 1, state, 0, 0, 1, 0, 1, 0, 0]);
            pcs.extend_from_slice(&x.to_be_bytes());
            pcs.extend_from_slice(&y.to_be_bytes());
            self.seg(pts, COMPOSITION, &pcs);
            self.seg(pts, WINDOW, &[1, 0, 0, 0, 0, 0, 0, 0, 0, 0]);
            self.seg(
                pts,
                PALETTE,
                &[0, 0, 1, 235, 128, 128, 255, 2, 16, 128, 128, 128],
            );
            let mut ods = vec![0, 1, 0, 0xc0, 0, 0, 0];
            ods.extend_from_slice(&ow.to_be_bytes());
            ods.extend_from_slice(&oh.to_be_bytes());
            ods.extend_from_slice(rle);
            self.seg(pts, OBJECT, &ods);
            self.seg(pts, END, &[]);
        }

        /// A display set placing `objects` (each `(x, y, width, height, rle)`, numbered from 1) with
        /// the same palette as [`Sup::show`], each object fragmented across segments as needed.
        #[cfg(test)]
        pub(crate) fn set(
            &mut self,
            pts: u32,
            state: u8,
            objects: &[(u16, u16, u16, u16, &[u8])],
            (cw, ch): (u16, u16),
        ) {
            let mut pcs = Vec::new();
            pcs.extend_from_slice(&cw.to_be_bytes());
            pcs.extend_from_slice(&ch.to_be_bytes());
            pcs.extend_from_slice(&[0x10, 0, 1, state, 0, 0, objects.len() as u8]);
            for (id, (x, y, ..)) in (1_u16..).zip(objects) {
                pcs.extend_from_slice(&id.to_be_bytes());
                pcs.extend_from_slice(&[0, 0]);
                pcs.extend_from_slice(&x.to_be_bytes());
                pcs.extend_from_slice(&y.to_be_bytes());
            }
            self.seg(pts, COMPOSITION, &pcs);
            self.seg(
                pts,
                PALETTE,
                &[0, 0, 1, 235, 128, 128, 255, 2, 16, 128, 128, 128],
            );
            for (id, (_, _, ow, oh, rle)) in (1_u16..).zip(objects) {
                let mut chunks = rle.chunks(60_000);
                let mut first = id.to_be_bytes().to_vec();
                first.extend_from_slice(&[0, 0xc0, 0, 0, 0]);
                first.extend_from_slice(&ow.to_be_bytes());
                first.extend_from_slice(&oh.to_be_bytes());
                first.extend_from_slice(chunks.next().unwrap_or_default());
                self.seg(pts, OBJECT, &first);
                for chunk in chunks {
                    let mut next = id.to_be_bytes().to_vec();
                    next.extend_from_slice(&[0, 0]);
                    next.extend_from_slice(chunk);
                    self.seg(pts, OBJECT, &next);
                }
            }
            self.seg(pts, END, &[]);
        }

        pub(crate) fn clear(&mut self, pts: u32, (cw, ch): (u16, u16)) {
            let mut pcs = Vec::new();
            pcs.extend_from_slice(&cw.to_be_bytes());
            pcs.extend_from_slice(&ch.to_be_bytes());
            pcs.extend_from_slice(&[0x10, 0, 2, 0, 0, 0, 0]);
            self.seg(pts, COMPOSITION, &pcs);
            self.seg(pts, WINDOW, &[1, 0, 0, 0, 0, 0, 0, 0, 0, 0]);
            self.seg(pts, END, &[]);
        }
    }

    /// Run-length codes one object row from `(length, index)` runs and ends it.
    #[cfg(test)]
    pub(crate) fn rle_row(runs: &[(u16, u8)], out: &mut Vec<u8>) {
        for &(len, index) in runs {
            match (len, index) {
                (0, _) => {}
                (1..=2, 1..) => out.extend(std::iter::repeat_n(index, usize::from(len))),
                (_, 0) if len < 64 => out.extend_from_slice(&[0, len as u8]),
                (_, 0) => out.extend_from_slice(&[0, 0x40 | (len >> 8) as u8, len as u8]),
                _ if len < 64 => out.extend_from_slice(&[0, 0x80 | len as u8, index]),
                _ => out.extend_from_slice(&[0, 0xc0 | (len >> 8) as u8, len as u8, index]),
            }
        }
        out.extend_from_slice(&[0, 0]);
    }
}

#[cfg(test)]
mod tests {
    use super::test_support::{Sup, TWO_ROWS};
    use super::*;

    const HD: (u16, u16) = (1920, 1080);

    fn decode_memory(
        bytes: &[u8],
        limits: Limits,
    ) -> Result<(u32, u32, Vec<StateResult>, Vec<image::RgbaImage>), WorkerError> {
        let mut memory = Memory::default();
        let Decoded {
            width: w,
            height: h,
            states,
            ..
        } = decode_pgs(bytes, limits, &mut memory)?;
        let images = memory
            .0
            .iter()
            .map(|png| {
                image::load_from_memory(png)
                    .expect("decode still")
                    .to_rgba8()
            })
            .collect();
        Ok((w, h, states, images))
    }

    /// Decodes keeping the stills encoded, returning the work and the number of stills written.
    fn decode_work(bytes: &[u8], limits: Limits) -> (Result<Decoded, WorkerError>, usize) {
        let mut memory = Memory::default();
        let decoded = decode_pgs(bytes, limits, &mut memory);
        (decoded, memory.0.len())
    }

    fn timing(states: &[StateResult]) -> Vec<(i64, i64, bool)> {
        states
            .iter()
            .map(|s| (s.start_ms, s.end_ms, s.still.is_some()))
            .collect()
    }

    #[test]
    fn each_display_set_is_drawn_on_the_canvas() {
        let mut w = Sup::default();
        w.show(90_000, EPOCH_START, 100, 900, &TWO_ROWS, 4, 2, HD);
        w.clear(360_000, HD);
        let (width, height, states, images) = decode_memory(&w.0, LIMITS).expect("decode");
        assert_eq!((width, height), (1920, 1080));
        assert_eq!(timing(&states), [(1000, 4000, true), (4000, 4000, false)]);
        let img = &images[0];
        assert_eq!(img.dimensions(), (1920, 1080));
        for (x, y, want) in [
            (100, 900, [0, 0, 0, 0]),
            (102, 900, [255, 255, 255, 255]),
            (103, 900, [255, 255, 255, 255]),
            (100, 901, [0, 0, 0, 128]),
            (103, 901, [0, 0, 0, 128]),
            (104, 900, [0, 0, 0, 0]),
        ] {
            assert_eq!(img.get_pixel(x, y).0, want, "pixel ({x},{y})");
        }
    }

    #[test]
    fn repeated_clears_fold() {
        let mut w = Sup::default();
        w.clear(0, HD);
        w.show(90_000, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, HD);
        w.clear(180_000, HD);
        w.clear(270_000, HD);
        let (_, _, states, _) = decode_memory(&w.0, LIMITS).expect("decode");
        assert_eq!(timing(&states), [(1000, 2000, true), (2000, 2000, false)]);
    }

    #[test]
    fn long_runs_span_their_width() {
        let rle = [0, 0xc0 | 1, (300 & 0xff) as u8, 1, 0, 0];
        let mut w = Sup::default();
        w.show(0, EPOCH_START, 10, 10, &rle, 300, 1, HD);
        let (_, _, _, images) = decode_memory(&w.0, LIMITS).expect("decode");
        let img = &images[0];
        assert_eq!(img.get_pixel(10, 10).0[3], 255);
        assert_eq!(img.get_pixel(309, 10).0[3], 255);
        assert_eq!(img.get_pixel(310, 10).0[3], 0);
    }

    #[test]
    fn a_truncated_segment_is_refused() {
        let mut w = Sup::default();
        w.show(0, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, HD);
        let Err(err) = decode_memory(&w.0[..w.0.len() - 5], LIMITS) else {
            panic!("a truncated stream decoded");
        };
        assert_eq!(err.code, "corrupt_input");
    }

    #[test]
    fn colours_round_trip_bt709() {
        assert_eq!(ycbcr_to_rgba(235, 128, 128, 255), [255, 255, 255, 255]);
        assert_eq!(ycbcr_to_rgba(16, 128, 128, 255), [0, 0, 0, 255]);
        let red = ycbcr_to_rgba(63, 240, 102, 255);
        assert!(red[0] >= 250 && red[1] <= 5 && red[2] <= 5, "red = {red:?}");
    }

    #[test]
    fn the_self_test_probe_passes() {
        assert!(self_test());
    }

    #[test]
    fn a_state_at_the_same_instant_replaces_the_one_before() {
        let mut w = Sup::default();
        w.show(90_000, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, HD);
        w.clear(90_000, HD);
        w.show(180_000, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, HD);
        let (_, _, states, images) = decode_memory(&w.0, LIMITS).expect("decode");
        assert_eq!(timing(&states), [(2000, 2000, true)]);
        assert_eq!(images.len(), 1);
    }

    #[test]
    fn states_stay_in_order_when_a_replacement_removes_one() {
        // The clear at 3 s replaces the show at 3 s and folds into the clear at 2 s, leaving
        // nothing pending; the show at 1.5 s still goes back in time.
        let mut w = Sup::default();
        for (ms, show) in [
            (1000, true),
            (2000, false),
            (3000, true),
            (3000, false),
            (1500, true),
            (4000, false),
        ] {
            if show {
                w.show(ms * 90, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, (8, 4));
            } else {
                w.clear(ms * 90, (8, 4));
            }
        }
        let Err(err) = decode_memory(&w.0, LIMITS) else {
            panic!("a track that goes back in time decoded");
        };
        assert_eq!(err.code, "corrupt_input", "{}", err.message);
    }

    #[test]
    fn a_replaced_state_is_never_encoded() {
        let mut w = Sup::default();
        for _ in 0..64 {
            w.show(90_000, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, (8, 4));
        }
        w.clear(180_000, (8, 4));
        let (decoded, stills) = decode_work(&w.0, LIMITS);
        let decoded = decoded.expect("decode");
        assert_eq!(
            timing(&decoded.states),
            [(1000, 2000, true), (2000, 2000, false)]
        );
        assert_eq!(stills, 1);
        // 63 replacements clear the canvas, one still is encoded, and the final clear clears it.
        assert_eq!(decoded.work.canvas_pixels, (63 + 1 + 1) * 32);
        assert_eq!(decoded.work.drawn_pixels, 64 * (8 + TWO_ROWS.len() as u64));
    }

    #[test]
    fn clearing_a_clean_canvas_costs_no_work() {
        let uhd = (3840, 2160);
        let mut clears = Sup::default();
        for _ in 0..2000 {
            clears.clear(0, uhd);
        }
        let (decoded, _) = decode_work(&clears.0, LIMITS);
        let decoded = decoded.expect("decode");
        assert!(decoded.states.is_empty());
        assert_eq!(decoded.work, Work::default());

        let mut w = Sup::default();
        w.show(0, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, (8, 4));
        for i in 1..2000 {
            w.clear(i * 90, (8, 4));
        }
        let (decoded, _) = decode_work(&w.0, LIMITS);
        let decoded = decoded.expect("decode");
        assert_eq!(timing(&decoded.states), [(0, 1, true), (1, 1, false)]);
        assert_eq!(decoded.work.canvas_pixels, 2 * 32);
    }

    #[test]
    fn work_beyond_a_ceiling_is_refused_before_it_is_done() {
        // One full-canvas object placed 64 times, then the same placements again at the same
        // instant: every set replaces the last, so none is ever final.
        let (side, canvas) = (64_u16, (64, 64));
        let mut rle = Vec::new();
        for _ in 0..side {
            test_support::rle_row(&[(side, 1)], &mut rle);
        }
        let placement = (0, 0, side, side, &rle[..]);
        let mut w = Sup::default();
        for _ in 0..3 {
            w.set(0, EPOCH_START, &[placement; 64], canvas);
        }
        let per_set = 64 * (u64::from(side) * u64::from(side) + rle.len() as u64);
        let drawn = Limits {
            max_drawn_pixels: 2 * per_set + 1,
            ..LIMITS
        };
        let (decoded, stills) = decode_work(&w.0, drawn);
        let err = decoded.err().expect("over-ceiling drawing decoded");
        assert_eq!(err.code, "limit_exceeded", "{}", err.message);
        assert_eq!(stills, 0);

        // Distinct instants: the second encode would pass the canvas ceiling.
        let mut w = Sup::default();
        for i in 0..3 {
            w.set(i * 90_000, EPOCH_START, &[placement], canvas);
        }
        let area = 64 * 64;
        let encoded = Limits {
            max_canvas_pixels: 2 * area,
            ..LIMITS
        };
        let (decoded, stills) = decode_work(&w.0, encoded);
        let err = decoded.err().expect("over-ceiling encoding decoded");
        assert_eq!(err.code, "limit_exceeded", "{}", err.message);
        assert_eq!(stills, 1);
    }

    /// A still sink that keeps only the encoded total.
    #[derive(Default)]
    struct Discard(u64);

    impl StillSink for Discard {
        fn write(&mut self, index: usize, png: &[u8]) -> Result<String, WorkerError> {
            self.0 += png.len() as u64;
            Ok(format!("s{index:05}.png"))
        }
    }

    /// A heavy realistic track: 3,000 display sets of two full-width objects of text strokes, each
    /// cleared before the next. `scale` 1 is 1920x1080 with 1920x200 objects; 2 is 3840x2160 with
    /// 3840x400 objects and strokes twice as long.
    fn heavy_track(scale: u16) -> Sup {
        let (cw, ch) = (1920 * scale, 1080 * scale);
        let (ow, oh) = (cw, 200 * scale);
        let mut seed = 0x9e37_79b9_u32;
        let mut next = |below: u16| {
            seed ^= seed << 13;
            seed ^= seed >> 17;
            seed ^= seed << 5;
            (seed % u32::from(below)) as u16
        };
        let mut w = Sup::default();
        for i in 0..3000_u32 {
            let objects: Vec<Vec<u8>> = (0..2)
                .map(|_| {
                    let mut rle = Vec::new();
                    for row in 0..oh {
                        if row < oh * 3 / 10 || row >= oh * 7 / 10 {
                            test_support::rle_row(&[], &mut rle);
                            continue;
                        }
                        let mut runs = vec![(ow / 4 + next(ow / 8), 0)];
                        let mut x = runs[0].0;
                        while x < ow * 3 / 4 {
                            let stroke = [
                                (2 * scale, 2),
                                ((3 + next(4)) * scale, 1),
                                (2 * scale, 2),
                                ((20 + next(30)) * scale, 0),
                            ];
                            x += stroke.iter().map(|(len, _)| len).sum::<u16>();
                            runs.extend(stroke);
                        }
                        test_support::rle_row(&runs, &mut rle);
                    }
                    rle
                })
                .collect();
            let ms = i * 4000;
            w.set(
                ms * 90,
                EPOCH_START,
                &[
                    (0, ch - 2 * oh - 20 * scale, ow, oh, &objects[0][..]),
                    (0, ch - oh - 10 * scale, ow, oh, &objects[1][..]),
                ],
                (cw, ch),
            );
            w.clear((ms + 2500) * 90, (cw, ch));
        }
        w
    }

    /// The measurement behind `DRAWN_PIXELS_CEILING` and `CANVAS_PIXELS_CEILING`. Run it in a
    /// release build: `cargo test --release -p loomarr-image -- --ignored --nocapture heavy`.
    #[test]
    #[ignore = "a release-build measurement of two full-length tracks"]
    fn heavy_realistic_tracks_fit_the_work_ceilings() {
        // The 3840x2160 track writes about 1 GB of stills, past the output ceiling; that ceiling
        // is lifted here so the work ceilings are measured on their own.
        let limits = Limits {
            max_output_bytes: u64::MAX,
            ..LIMITS
        };
        for scale in [1, 2] {
            let track = heavy_track(scale);
            let started = std::time::Instant::now();
            let mut sink = Discard::default();
            let decoded = decode_pgs(&track.0, limits, &mut sink).expect("decode");
            println!(
                "scale {scale}: {} input bytes, {} states, {:?}, {} output bytes, {:?}",
                track.0.len(),
                decoded.states.len(),
                decoded.work,
                sink.0,
                started.elapsed(),
            );
        }
    }

    #[test]
    fn malformed_and_over_ceiling_tracks_are_refused_with_stable_codes() {
        let mut backwards = Sup::default();
        backwards.show(180_000, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, HD);
        backwards.show(90_000, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, HD);
        let mut resized = Sup::default();
        resized.show(0, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, HD);
        resized.clear(90_000, (1280, 720));
        let mut wide = Sup::default();
        wide.clear(0, (3841, 2160));
        let mut big_object = Sup::default();
        big_object.show(0, EPOCH_START, 0, 0, &TWO_ROWS, 3841, 2, HD);
        let mut crowded = Sup::default();
        let mut pcs = vec![0x07, 0x80, 0x04, 0x38, 0x10, 0, 1, EPOCH_START, 0, 0, 65];
        pcs.extend(std::iter::repeat_n([0, 1, 0, 0, 0, 0, 0, 0], 65).flatten());
        crowded.seg(0, COMPOSITION, &pcs);
        let mut truncated_run = Sup::default();
        truncated_run.show(0, EPOCH_START, 0, 0, &[0, 0x40], 4, 2, HD);
        let mut many = Sup::default();
        for i in 0..3_u32 {
            many.show(i * 180_000, EPOCH_START, 0, 0, &TWO_ROWS, 4, 2, (8, 4));
            many.clear(i * 180_000 + 90_000, (8, 4));
        }
        let three_states = Limits {
            max_states: 3,
            ..LIMITS
        };
        let tiny_output = Limits {
            max_output_bytes: 10,
            ..LIMITS
        };
        for (name, bytes, limits, code) in [
            ("garbage", b"not a sup".to_vec(), LIMITS, "corrupt_input"),
            ("empty", Vec::new(), LIMITS, "corrupt_input"),
            ("backwards", backwards.0, LIMITS, "corrupt_input"),
            ("resized", resized.0, LIMITS, "corrupt_input"),
            ("truncated run", truncated_run.0, LIMITS, "corrupt_input"),
            ("wide canvas", wide.0, LIMITS, "limit_exceeded"),
            ("big object", big_object.0, LIMITS, "limit_exceeded"),
            ("65 objects", crowded.0, LIMITS, "limit_exceeded"),
            ("states", many.0.clone(), three_states, "limit_exceeded"),
            ("output", many.0, tiny_output, "limit_exceeded"),
        ] {
            let err = decode_memory(&bytes, limits)
                .err()
                .unwrap_or_else(|| panic!("{name} decoded"));
            assert_eq!(err.code, code, "{name}: {}", err.message);
        }
    }

    #[test]
    fn the_request_writes_stills_into_staging_and_refuses_other_codecs() {
        /// Removes the directory when dropped, so a failing assertion leaves nothing behind.
        struct TempDir(PathBuf);
        impl Drop for TempDir {
            fn drop(&mut self) {
                let _ = fs::remove_dir_all(&self.0);
            }
        }
        let dir =
            TempDir(std::env::temp_dir().join(format!("loomarr-subs-{}", std::process::id())));
        let root = &dir.0;
        let staging = root.join("staging");
        fs::create_dir_all(&staging).expect("staging");
        let mut w = Sup::default();
        w.show(90_000, EPOCH_START, 2, 1, &TWO_ROWS, 4, 2, (8, 4));
        w.clear(360_000, (8, 4));
        let track = root.join("track.sup");
        fs::write(&track, &w.0).expect("track");
        let request = |codec: &str| {
            serde_json::to_vec(&serde_json::json!({
                "protocol": 1, "requestId": "r",
                "source": {"codec": codec, "path": track, "expectedSha256": hex_digest(&w.0)},
                "stagingDir": staging,
            }))
            .expect("request")
        };
        let mut out = Vec::new();
        run_decode_subtitles(&request(PGS_CODEC)[..], &mut out).expect("decode");
        let result: serde_json::Value = serde_json::from_slice(&out).expect("result");
        assert_eq!(result["width"], 8);
        assert_eq!(result["states"][0]["still"]["relativePath"], "s00000.png");
        assert!(result["states"][1]["still"].is_null());
        let png = fs::read(staging.join("s00000.png")).expect("still");
        assert_eq!(result["states"][0]["still"]["sha256"], hex_digest(&png));

        let mut out = Vec::new();
        let err = run_decode_subtitles(&request("subrip")[..], &mut out).expect_err("refused");
        assert_eq!(err.code, "unsupported_input");
        let refusal: serde_json::Value = serde_json::from_slice(&out).expect("refusal");
        assert_eq!(refusal["error"]["code"], "unsupported_input");
    }
}
