//! `decode-subtitles --protocol 1`: an image subtitle track in, timed full-canvas stills out.
//!
//! A codec decoder reports each display state to a [`Timeline`] as a start time and a full
//! straight-alpha RGBA canvas (or `None` when nothing shows). The timeline is codec-neutral: it
//! folds repeated clears, enforces the state and output ceilings, and writes one PNG per still into
//! staging. PGS (`hdmv_pgs_subtitle`) is the one decoder; VOBSUB (#2124) adds a second behind the
//! same timeline.

use std::collections::HashMap;
use std::fs;
use std::io::Read;
use std::path::PathBuf;

use image::ImageEncoder;
use image::codecs::png::{CompressionType, FilterType, PngEncoder};
use serde::{Deserialize, Serialize};

use crate::{PROTOCOL, WorkerError, hex_digest, read_control, write_complete, write_error};

/// The hard ceilings of design `images.md` *Operations beyond renditions*.
#[derive(Clone, Copy, Debug)]
pub(crate) struct Limits {
    pub max_input_bytes: u64,
    pub max_width: u32,
    pub max_height: u32,
    pub max_objects_per_state: usize,
    pub max_states: usize,
    pub max_output_bytes: u64,
}

pub(crate) const LIMITS: Limits = Limits {
    max_input_bytes: 256 << 20,
    max_width: 3840,
    max_height: 2160,
    max_objects_per_state: 64,
    max_states: 50_000,
    max_output_bytes: 512 << 20,
};

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
/// same instant as the one before it replaces it, so a PNG is encoded only once its time is final.
pub(crate) struct Timeline<'a, S: StillSink> {
    limits: Limits,
    sink: &'a mut S,
    states: Vec<StateResult>,
    pending: Option<(i64, Option<Vec<u8>>)>,
    output_bytes: u64,
}

impl<'a, S: StillSink> Timeline<'a, S> {
    pub(crate) fn new(limits: Limits, sink: &'a mut S) -> Self {
        Self {
            limits,
            sink,
            states: Vec::new(),
            pending: None,
            output_bytes: 0,
        }
    }

    pub(crate) fn push(
        &mut self,
        start_ms: i64,
        canvas: Option<&Canvas>,
    ) -> Result<(), WorkerError> {
        if let Some((pending_start, _)) = &self.pending {
            if start_ms < *pending_start {
                return Err(WorkerError::new(
                    "corrupt_input",
                    "display states go back in time",
                ));
            }
            if start_ms > *pending_start {
                self.flush()?;
            } else {
                self.pending = None;
            }
        }
        let previous_clear = match &self.pending {
            Some((_, still)) => still.is_none(),
            None => self.states.last().is_none_or(|state| state.still.is_none()),
        };
        if canvas.is_none() && previous_clear {
            return Ok(());
        }
        let png = canvas.map(encode_png).transpose()?;
        self.pending = Some((start_ms, png));
        Ok(())
    }

    fn flush(&mut self) -> Result<(), WorkerError> {
        let Some((start_ms, png)) = self.pending.take() else {
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
        let still = match png {
            None => None,
            Some(png) => {
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
            }
        };
        self.states.push(StateResult {
            start_ms,
            end_ms: start_ms,
            still,
        });
        Ok(())
    }

    fn finish(mut self) -> Result<Vec<StateResult>, WorkerError> {
        self.flush()?;
        Ok(self.states)
    }
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
    if request.source.codec != PGS_CODEC {
        return Err(WorkerError::new(
            "unsupported_input",
            "subtitle codec is not decodable",
        ));
    }
    let meta = fs::symlink_metadata(&request.source.path)
        .map_err(|err| WorkerError::new("io_failed", format!("stat source: {err}")))?;
    if !meta.is_file() {
        return Err(WorkerError::new(
            "invalid_request",
            "source is not a regular file",
        ));
    }
    if meta.len() > limits.max_input_bytes {
        return Err(WorkerError::new(
            "limit_exceeded",
            "track exceeds the input ceiling",
        ));
    }
    let mut source = Vec::new();
    fs::File::open(&request.source.path)
        .and_then(|file| {
            file.take(limits.max_input_bytes + 1)
                .read_to_end(&mut source)
        })
        .map_err(|err| WorkerError::new("io_failed", format!("read source: {err}")))?;
    if source.len() as u64 > limits.max_input_bytes {
        return Err(WorkerError::new(
            "limit_exceeded",
            "track exceeds the input ceiling",
        ));
    }
    let sha256 = hex_digest(&source);
    if sha256 != request.source.expected_sha256 {
        return Err(WorkerError::new(
            "source_changed",
            "source SHA-256 does not match request",
        ));
    }
    let mut staging = Staging(request.staging_dir);
    let (width, height, states) = decode_pgs(&source, limits, &mut staging)?;
    Ok(DecodeResult {
        protocol: PROTOCOL,
        status: "ok",
        request_id: request.request_id,
        source: TrackResult {
            codec: PGS_CODEC,
            sha256,
            bytes: source.len() as u64,
        },
        width,
        height,
        states,
    })
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
) -> Result<(u32, u32, Vec<StateResult>), WorkerError> {
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
                        canvas = Some(Canvas {
                            width,
                            height,
                            pixels: vec![0; width as usize * height as usize * 4],
                        });
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
                let drawn = render(canvas, &placed, &objects, palettes.get(&palette_id))?;
                timeline.push(start_ms, drawn.then_some(&*canvas))?;
            }
            _ => {}
        }
    }
    let Some(canvas) = canvas else {
        return Err(corrupt("track has no compositions"));
    };
    Ok((canvas.width, canvas.height, timeline.finish()?))
}

/// Draws one composition onto the cleared canvas; false when nothing shows.
fn render(
    canvas: &mut Canvas,
    placed: &[Placement],
    objects: &HashMap<u16, Object>,
    palette: Option<&[[u8; 4]; 256]>,
) -> Result<bool, WorkerError> {
    canvas.pixels.fill(0);
    let Some(palette) = palette else {
        return Ok(false);
    };
    let mut drawn = false;
    for placement in placed {
        // An acquisition point can name an object a later set redefines; skip it.
        let Some(object) = objects.get(&placement.object) else {
            continue;
        };
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
            let from = from.max(x0);
            let to = to.min(x1);
            for x in from..to {
                let canvas_x = u64::from(placement.x) + u64::from(x - x0);
                if canvas_x >= u64::from(canvas.width) {
                    break;
                }
                let at = ((canvas_y * u64::from(canvas.width) + canvas_x) * 4) as usize;
                canvas.pixels[at..at + 4].copy_from_slice(&colour);
                drawn = true;
            }
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

/// The fuzz entry: the PGS decoder over arbitrary bytes, stills kept in memory.
#[doc(hidden)]
pub fn fuzz_decode_pgs(bytes: &[u8]) -> Result<usize, String> {
    let limits = Limits {
        max_width: 256,
        max_height: 256,
        max_states: 64,
        max_output_bytes: 4 << 20,
        ..LIMITS
    };
    decode_pgs(bytes, limits, &mut Memory::default())
        .map(|(_, _, states)| states.len())
        .map_err(|err| err.code.to_owned())
}

#[derive(Default)]
pub(crate) struct Memory(pub Vec<Vec<u8>>);

impl StillSink for Memory {
    fn write(&mut self, index: usize, png: &[u8]) -> Result<String, WorkerError> {
        self.0.push(png.to_vec());
        Ok(format!("s{index:05}.png"))
    }
}

/// A 4x2 PGS probe: one still at 1 s, cleared at 4 s, on an 8x4 canvas.
pub(crate) fn self_test() -> bool {
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
    let Ok((8, 4, states)) = decode_pgs(&w.0, LIMITS, &mut memory) else {
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
        let (w, h, states) = decode_pgs(bytes, limits, &mut memory)?;
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
        let root = std::env::temp_dir().join(format!("loomarr-subs-{}", std::process::id()));
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
        let err =
            run_decode_subtitles(&request("dvd_subtitle")[..], &mut out).expect_err("refused");
        assert_eq!(err.code, "unsupported_input");
        let refusal: serde_json::Value = serde_json::from_slice(&out).expect("refusal");
        assert_eq!(refusal["error"]["code"], "unsupported_input");
        fs::remove_dir_all(root).expect("cleanup");
    }
}
