//! VOBSUB (`dvd_subtitle`): a DVD subpicture track extracted as an `.idx`/`.sub` pair.
//!
//! The `.idx` gives the canvas size, the 16-colour palette, optional custom colours, the
//! forced-only flag and, for each subpicture, a timestamp and the `.sub` offset of the pack it
//! starts in. The `.sub` is an MPEG-2 program stream whose private stream 1 packets carry each
//! subpicture unit (SPU) in pieces; one linear pass reassembles them. An SPU's control sequences
//! say when it starts and stops showing, which palette entries and alphas its four pixel values
//! take, and where its two interlaced fields of 2-bit run-length coded rows sit on the canvas.

use std::collections::{HashMap, HashSet};

use super::{Budget, Canvas, Decoded, Limits, StillSink, Timeline, be16, corrupt};
use crate::WorkerError;

pub const VOBSUB_CODEC: &str = "dvd_subtitle";

// Program stream start codes.
const PACK: u8 = 0xba;
const PROGRAM_END: u8 = 0xb9;
const PRIVATE_STREAM_1: u8 = 0xbd;
/// The first subpicture substream; an `.idx` track `index: N` is substream `0x20 + N`.
const FIRST_SUBPICTURE: u8 = 0x20;

// SPU control commands.
const FORCED_START: u8 = 0x00;
const START: u8 = 0x01;
const STOP: u8 = 0x02;
const COLOUR: u8 = 0x03;
const ALPHA: u8 = 0x04;
const AREA: u8 = 0x05;
const FIELDS: u8 = 0x06;
const CHANGE_COLOUR_CONTRAST: u8 = 0x07;
const SEQUENCE_END: u8 = 0xff;

/// A parsed `.idx`: one track's canvas, colours and entries.
struct Index {
    width: u32,
    height: u32,
    palette: [[u8; 3]; 16],
    custom: Option<Custom>,
    forced_only: bool,
    stream: u8,
    entries: Vec<Entry>,
}

/// `custom colors: ON`: the four listed colours replace the palette lookup for pixel values 0..3
/// (background, pattern, emphasis 1, emphasis 2), and a `1` in the same place in `tridx` makes
/// that value transparent.
struct Custom {
    colours: [[u8; 3]; 4],
    transparent: [bool; 4],
}

struct Entry {
    ms: i64,
    filepos: u64,
}

/// What an SPU's control sequences set by the time it starts showing. Delays are in SPU ticks of
/// 1024 periods of the 90 kHz clock.
#[derive(Default)]
struct Subpicture {
    start: Option<u16>,
    stop: Option<u16>,
    forced: bool,
    /// The palette entry and alpha (0..15) of pixel values 0..3.
    colour: [u8; 4],
    alpha: [u8; 4],
    /// `(x1, y1, x2, y2)`, inclusive.
    area: Option<(u32, u32, u32, u32)>,
    /// The byte offsets of the top (even rows) and bottom (odd rows) fields.
    fields: Option<(usize, usize)>,
}

fn ticks_ms(ticks: u16) -> i64 {
    (i64::from(ticks) * 1024 + 45) / 90
}

/// Decodes a pair into the canvas size and its states. A subpicture starts at its entry's
/// timestamp plus its start delay and stops at its stop delay; a later subpicture that starts
/// first replaces it, and one with no stop holds until the next. With `forced subs: ON` only
/// subpictures marked forced show.
pub(super) fn decode_vobsub<S: StillSink>(
    idx: &[u8],
    sub: &[u8],
    limits: Limits,
    sink: &mut S,
) -> Result<Decoded, WorkerError> {
    let index = parse_index(idx, limits)?;
    let wanted = index.entries.iter().map(|entry| entry.filepos).collect();
    let units = reassemble(sub, index.stream, &wanted)?;
    let mut shown = Vec::new();
    for entry in &index.entries {
        let Some(unit) = units.get(&entry.filepos) else {
            return Err(corrupt("an index entry points at no complete subpicture"));
        };
        let picture = parse_spu(unit)?;
        let Some(start) = picture.start else {
            continue;
        };
        if index.forced_only && !picture.forced {
            continue;
        }
        let start_ms = (entry.ms + ticks_ms(start)).max(0);
        let stop_ms = picture
            .stop
            .map(|stop| (entry.ms + ticks_ms(stop)).max(start_ms));
        shown.push((start_ms, stop_ms, unit.as_slice(), picture));
    }
    let mut canvas = Canvas::new(index.width, index.height);
    let mut timeline = Timeline::new(limits, sink);
    for (i, (start_ms, stop_ms, unit, picture)) in shown.iter().enumerate() {
        let colours = colours(&index, picture);
        timeline.push(*start_ms, &mut canvas, |canvas, budget| {
            render(canvas, budget, unit, picture, &colours, limits)
        })?;
        let next = shown.get(i + 1).map(|(next, ..)| *next);
        if let Some(stop_ms) = stop_ms
            && next.is_none_or(|next| *stop_ms < next)
        {
            timeline.push(*stop_ms, &mut canvas, |_, _| Ok(false))?;
        }
    }
    let (states, work) = timeline.finish(&canvas)?;
    Ok(Decoded {
        width: canvas.width,
        height: canvas.height,
        states,
        work,
    })
}

fn parse_index(idx: &[u8], limits: Limits) -> Result<Index, WorkerError> {
    let text = std::str::from_utf8(idx).map_err(|_| corrupt("index is not text"))?;
    let mut size = None;
    let mut palette = None;
    let mut custom = None;
    let mut forced_only = false;
    let mut stream = None;
    let mut offset_ms = 0_i64;
    let mut delay_ms = 0_i64;
    let mut entries = Vec::new();
    for line in text.lines() {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        let Some((key, value)) = line.split_once(':') else {
            continue;
        };
        let value = value.trim();
        match key.trim() {
            "size" => size = Some(parse_size(value, limits)?),
            "palette" => palette = Some(parse_palette(value)?),
            "custom colors" => custom = parse_custom(value)?,
            "forced subs" => forced_only = value.eq_ignore_ascii_case("on"),
            "time offset" => offset_ms = parse_offset(value)?,
            // A delay shifts every later timestamp of the track; successive delays add up.
            "delay" => delay_ms = delay_ms.saturating_add(parse_offset(value)?),
            "id" => {
                if stream.is_some() {
                    return Err(WorkerError::new(
                        "unsupported_input",
                        "the index holds more than one track",
                    ));
                }
                let number = value
                    .split_once("index:")
                    .and_then(|(_, n)| n.trim().parse::<u8>().ok())
                    .filter(|n| *n < 32)
                    .ok_or_else(|| corrupt("bad track id"))?;
                stream = Some(FIRST_SUBPICTURE + number);
            }
            "timestamp" => {
                if stream.is_none() {
                    return Err(corrupt("a timestamp precedes the track id"));
                }
                if entries.len() >= limits.max_states {
                    return Err(WorkerError::new(
                        "limit_exceeded",
                        "index entries exceed the state ceiling",
                    ));
                }
                let (time, filepos) = value
                    .split_once(',')
                    .ok_or_else(|| corrupt("bad timestamp"))?;
                let filepos = filepos
                    .trim()
                    .strip_prefix("filepos:")
                    .and_then(|pos| u64::from_str_radix(pos.trim(), 16).ok())
                    .ok_or_else(|| corrupt("bad filepos"))?;
                let ms = parse_time(time.trim())?
                    .saturating_add(delay_ms)
                    .saturating_add(offset_ms);
                entries.push(Entry { ms, filepos });
            }
            _ => {}
        }
    }
    let (width, height) = size.ok_or_else(|| corrupt("index has no size"))?;
    let palette = palette.ok_or_else(|| corrupt("index has no palette"))?;
    Ok(Index {
        width,
        height,
        palette,
        custom,
        forced_only,
        stream: stream.unwrap_or(FIRST_SUBPICTURE),
        entries,
    })
}

fn parse_size(value: &str, limits: Limits) -> Result<(u32, u32), WorkerError> {
    let (w, h) = value
        .split_once('x')
        .and_then(|(w, h)| Some((w.trim().parse::<u32>().ok()?, h.trim().parse::<u32>().ok()?)))
        .ok_or_else(|| corrupt("bad size"))?;
    if w == 0 || h == 0 {
        return Err(corrupt("empty canvas"));
    }
    if w > limits.max_width || h > limits.max_height {
        return Err(WorkerError::new(
            "limit_exceeded",
            "canvas exceeds the ceiling",
        ));
    }
    Ok((w, h))
}

fn parse_rgb(value: &str) -> Result<[u8; 3], WorkerError> {
    let value = value.trim();
    if value.len() != 6 {
        return Err(corrupt("bad colour"));
    }
    let rgb = u32::from_str_radix(value, 16).map_err(|_| corrupt("bad colour"))?;
    let [_, r, g, b] = rgb.to_be_bytes();
    Ok([r, g, b])
}

fn parse_palette(value: &str) -> Result<[[u8; 3]; 16], WorkerError> {
    let mut palette = [[0; 3]; 16];
    let mut count = 0;
    for colour in value.split(',') {
        let Some(slot) = palette.get_mut(count) else {
            return Err(corrupt("palette has more than 16 colours"));
        };
        *slot = parse_rgb(colour)?;
        count += 1;
    }
    if count != 16 {
        return Err(corrupt("palette has fewer than 16 colours"));
    }
    Ok(palette)
}

/// `ON, tridx: 1000, colors: 000000, 828282, 828282, 828282`; `OFF` keeps the palette.
fn parse_custom(value: &str) -> Result<Option<Custom>, WorkerError> {
    let mut parts = value.split(',').map(str::trim);
    if !parts.next().is_some_and(|on| on.eq_ignore_ascii_case("on")) {
        return Ok(None);
    }
    let tridx = parts
        .next()
        .and_then(|part| part.strip_prefix("tridx:"))
        .map(str::trim)
        .filter(|digits| digits.len() == 4 && digits.bytes().all(|d| d == b'0' || d == b'1'))
        .ok_or_else(|| corrupt("bad custom colour tridx"))?;
    let first = parts
        .next()
        .and_then(|part| part.strip_prefix("colors:"))
        .ok_or_else(|| corrupt("bad custom colours"))?;
    let mut colours = [[0; 3]; 4];
    colours[0] = parse_rgb(first)?;
    for slot in &mut colours[1..] {
        *slot = parse_rgb(parts.next().ok_or_else(|| corrupt("bad custom colours"))?)?;
    }
    let mut transparent = [false; 4];
    for (slot, digit) in transparent.iter_mut().zip(tridx.bytes()) {
        *slot = digit == b'1';
    }
    Ok(Some(Custom {
        colours,
        transparent,
    }))
}

/// `hh:mm:ss:ms` in milliseconds.
fn parse_time(value: &str) -> Result<i64, WorkerError> {
    let bad = || corrupt("bad time");
    let mut parts = value.split(':');
    let mut field = |max: i64| -> Result<i64, WorkerError> {
        let part = parts.next().ok_or_else(bad)?;
        if part.is_empty() || part.len() > 6 || !part.bytes().all(|d| d.is_ascii_digit()) {
            return Err(bad());
        }
        let n: i64 = part.parse().map_err(|_| bad())?;
        if n > max {
            return Err(bad());
        }
        Ok(n)
    };
    let (h, m, s, ms) = (field(999_999)?, field(59)?, field(59)?, field(999)?);
    if parts.next().is_some() {
        return Err(bad());
    }
    Ok(((h * 60 + m) * 60 + s) * 1000 + ms)
}

/// A signed `hh:mm:ss:ms`, or a signed whole number of milliseconds.
fn parse_offset(value: &str) -> Result<i64, WorkerError> {
    let (sign, rest) = match value.strip_prefix('-') {
        Some(rest) => (-1, rest.trim()),
        None => (1, value.strip_prefix('+').unwrap_or(value).trim()),
    };
    let ms = if rest.contains(':') {
        parse_time(rest)?
    } else {
        if rest.is_empty() || rest.len() > 12 || !rest.bytes().all(|d| d.is_ascii_digit()) {
            return Err(corrupt("bad time offset"));
        }
        rest.parse().map_err(|_| corrupt("bad time offset"))?
    };
    Ok(sign * ms)
}

/// Reassembles each complete SPU of `stream` that starts in a pack at one of `wanted`, keyed by
/// that pack's offset. Other units are only counted through, so memory stays bounded by the index
/// whatever the `.sub` holds. A unit cut off at the end of the file is dropped.
fn reassemble(
    mut bytes: &[u8],
    stream: u8,
    wanted: &HashSet<u64>,
) -> Result<HashMap<u64, Vec<u8>>, WorkerError> {
    let total = bytes.len();
    let mut units = HashMap::new();
    let mut pack_at = 0_u64;
    // The unit being reassembled: the pack it started in, its bytes if wanted, how many it has
    // and its declared size.
    let mut building: Option<(u64, Option<Vec<u8>>, usize, usize)> = None;
    while !bytes.is_empty() {
        let at = (total - bytes.len()) as u64;
        if bytes.len() < 4 || bytes[..3] != [0, 0, 1] {
            return Err(corrupt("bad start code"));
        }
        match bytes[3] {
            PACK => {
                if bytes.len() < 14 {
                    return Err(corrupt("truncated pack header"));
                }
                if bytes[4] & 0xc0 != 0x40 {
                    return Err(corrupt("not an MPEG-2 program stream"));
                }
                let header = 14 + usize::from(bytes[13] & 0x07);
                if bytes.len() < header {
                    return Err(corrupt("truncated pack header"));
                }
                pack_at = at;
                bytes = &bytes[header..];
            }
            PROGRAM_END => bytes = &bytes[4..],
            0xbb.. => {
                if bytes.len() < 6 {
                    return Err(corrupt("truncated packet"));
                }
                let len = be16(&bytes[4..6]) as usize;
                if bytes.len() < 6 + len {
                    return Err(corrupt("truncated packet"));
                }
                let packet = &bytes[6..6 + len];
                let kind = bytes[3];
                bytes = &bytes[6 + len..];
                if kind != PRIVATE_STREAM_1 {
                    continue;
                }
                if packet.len() < 3 || packet[0] & 0xc0 != 0x80 {
                    return Err(corrupt("bad PES header"));
                }
                let Some(payload) = packet.get(3 + usize::from(packet[2])..) else {
                    return Err(corrupt("bad PES header"));
                };
                let Some((&id, data)) = payload.split_first() else {
                    continue;
                };
                if id != stream {
                    continue;
                }
                let (start, mut unit, got, size) = match building.take() {
                    Some(unit) => unit,
                    None => {
                        if data.len() < 2 {
                            return Err(corrupt("short subpicture"));
                        }
                        let size = be16(data) as usize;
                        if size < 4 {
                            return Err(corrupt("short subpicture"));
                        }
                        let keep = wanted.contains(&pack_at).then(|| Vec::with_capacity(size));
                        (pack_at, keep, 0, size)
                    }
                };
                let piece = &data[..data.len().min(size - got)];
                if let Some(unit) = &mut unit {
                    unit.extend_from_slice(piece);
                }
                let got = got + piece.len();
                match unit {
                    Some(unit) if got == size => {
                        units.insert(start, unit);
                    }
                    _ if got == size => {}
                    unit => building = Some((start, unit, got, size)),
                }
            }
            _ => return Err(corrupt("bad start code")),
        }
    }
    Ok(units)
}

/// Reads an SPU's control sequences. Colours, alphas, the area and the field offsets are taken as
/// they stand at the end of the sequence that starts the display; later changes are ignored.
fn parse_spu(unit: &[u8]) -> Result<Subpicture, WorkerError> {
    let control = be16(&unit[2..4]) as usize;
    if control < 4 || control >= unit.len() {
        return Err(corrupt("bad subpicture control offset"));
    }
    let mut picture = Subpicture::default();
    let mut frozen = false;
    let mut at = control;
    loop {
        if unit.len() < at + 4 {
            return Err(corrupt("short control sequence"));
        }
        let delay = be16(&unit[at..at + 2]) as u16;
        let next = be16(&unit[at + 2..at + 4]) as usize;
        let mut i = at + 4;
        loop {
            let Some(&command) = unit.get(i) else {
                return Err(corrupt("unterminated control sequence"));
            };
            i += 1;
            let args = |n: usize| {
                unit.get(i..i + n)
                    .ok_or_else(|| corrupt("short control command"))
            };
            match command {
                FORCED_START | START => {
                    picture.forced |= command == FORCED_START && picture.start.is_none();
                    picture.start.get_or_insert(delay);
                }
                STOP => {
                    if picture.start.is_some() && picture.stop.is_none() {
                        picture.stop = Some(delay);
                    }
                }
                COLOUR | ALPHA => {
                    let b = args(2)?;
                    i += 2;
                    if !frozen {
                        let nibbles = [b[1] & 0x0f, b[1] >> 4, b[0] & 0x0f, b[0] >> 4];
                        if command == COLOUR {
                            picture.colour = nibbles;
                        } else {
                            picture.alpha = nibbles;
                        }
                    }
                }
                AREA => {
                    let b = args(6)?;
                    i += 6;
                    if !frozen {
                        let twelve = |hi: u8, lo: u8, shift: bool| {
                            if shift {
                                u32::from(hi) << 4 | u32::from(lo) >> 4
                            } else {
                                u32::from(hi & 0x0f) << 8 | u32::from(lo)
                            }
                        };
                        picture.area = Some((
                            twelve(b[0], b[1], true),
                            twelve(b[3], b[4], true),
                            twelve(b[1], b[2], false),
                            twelve(b[4], b[5], false),
                        ));
                    }
                }
                FIELDS => {
                    let b = args(4)?;
                    i += 4;
                    if !frozen {
                        picture.fields = Some((be16(&b[0..2]) as usize, be16(&b[2..4]) as usize));
                    }
                }
                CHANGE_COLOUR_CONTRAST => {
                    // Mid-display colour changes; the size counts its own two bytes.
                    let size = be16(args(2)?) as usize;
                    if size < 2 || unit.len() < i + size {
                        return Err(corrupt("short control command"));
                    }
                    i += size;
                }
                SEQUENCE_END => break,
                _ => return Err(corrupt("unknown subpicture command")),
            }
        }
        frozen |= picture.start.is_some();
        // The last sequence points at itself; a pointer backwards also ends the list.
        if next <= at {
            break;
        }
        at = next;
    }
    Ok(picture)
}

/// The straight-alpha RGBA of pixel values 0..3.
fn colours(index: &Index, picture: &Subpicture) -> [[u8; 4]; 4] {
    let mut out = [[0; 4]; 4];
    for (value, slot) in out.iter_mut().enumerate() {
        let alpha = picture.alpha[value] * 17;
        let [r, g, b] = match &index.custom {
            Some(custom) if custom.transparent[value] => continue,
            Some(custom) => custom.colours[value],
            None => index.palette[usize::from(picture.colour[value])],
        };
        *slot = [r, g, b, alpha];
    }
    out
}

/// Draws one subpicture onto the cleared canvas, charging its area and coded length before it
/// decodes; false when nothing shows.
fn render(
    canvas: &mut Canvas,
    budget: &mut Budget,
    unit: &[u8],
    picture: &Subpicture,
    colours: &[[u8; 4]; 4],
    limits: Limits,
) -> Result<bool, WorkerError> {
    let (Some((x1, y1, x2, y2)), Some((top, bottom))) = (picture.area, picture.fields) else {
        return Err(corrupt("a shown subpicture has no display area"));
    };
    if x2 < x1 || y2 < y1 {
        return Err(corrupt("inverted display area"));
    }
    let (width, height) = (x2 - x1 + 1, y2 - y1 + 1);
    if width > limits.max_width || height > limits.max_height {
        return Err(WorkerError::new(
            "limit_exceeded",
            "subpicture exceeds the canvas ceiling",
        ));
    }
    budget.draw(u64::from(width) * u64::from(height) + unit.len() as u64)?;
    if colours.iter().all(|colour| colour[3] == 0) {
        return Ok(false);
    }
    let mut drawn = false;
    let mut draw = |row: u32, from: u32, to: u32, value: u8| {
        let colour = colours[usize::from(value)];
        let canvas_y = u64::from(y1) + u64::from(row);
        if colour[3] == 0 || canvas_y >= u64::from(canvas.height) {
            return;
        }
        let left = u64::from(x1) + u64::from(from);
        let right = (u64::from(x1) + u64::from(to)).min(u64::from(canvas.width));
        if left >= right {
            return;
        }
        let row_at = canvas_y * u64::from(canvas.width);
        let span = ((row_at + left) * 4) as usize..((row_at + right) * 4) as usize;
        canvas.pixels[span].as_chunks_mut::<4>().0.fill(colour);
        drawn = true;
    };
    for (field, offset) in [(0, top), (1, bottom)] {
        let Some(data) = unit.get(offset..) else {
            return Err(corrupt("field offset is past the subpicture"));
        };
        decode_field(data, width, (field..height).step_by(2), &mut draw)?;
    }
    Ok(drawn)
}

/// Expands one field's rows of 2-bit run-length codes, reporting each run as
/// `(row, x0, x1, value)` with `x1` clipped to the width. A code's leading zero nibbles say its
/// length: `nncc` runs 1-3, `00nn nncc` 4-15, `0000 nnnn nncc` 16-63, and `0000 00nn nnnn nncc`
/// 64-255 or, with a zero length, the rest of the row. Rows end on a byte boundary. Rows the data
/// runs out before stay clear.
fn decode_field(
    data: &[u8],
    width: u32,
    rows: impl Iterator<Item = u32>,
    draw: &mut impl FnMut(u32, u32, u32, u8),
) -> Result<(), WorkerError> {
    let mut at = 0_usize;
    let nibble = |at: &mut usize| {
        let byte = *data.get(*at / 2)?;
        let n = if at.is_multiple_of(2) {
            byte >> 4
        } else {
            byte & 0x0f
        };
        *at += 1;
        Some(u32::from(n))
    };
    for row in rows {
        let mut x = 0;
        while x < width {
            let Some(mut code) = nibble(&mut at) else {
                return Ok(());
            };
            for threshold in [0x4, 0x10, 0x40] {
                if code >= threshold {
                    break;
                }
                let Some(n) = nibble(&mut at) else {
                    return Err(corrupt("truncated run"));
                };
                code = code << 4 | n;
            }
            let run = code >> 2;
            let end = if run == 0 {
                width
            } else {
                x.saturating_add(run).min(width)
            };
            draw(row, x, end, (code & 3) as u8);
            x = end;
        }
        at = at.next_multiple_of(2);
    }
    Ok(())
}

/// A 4x2 VOBSUB probe on an 8x4 canvas: one still at 1 s, stopped 512 ms later.
pub(super) fn self_test() -> bool {
    let mut pair = test_support::Pair::new((8, 4), "");
    pair.spu(1000, &test_support::Unit::two_rows(2, 1).bytes(), 2048);
    let mut memory = super::Memory::default();
    let Ok(Decoded {
        width: 8,
        height: 4,
        states,
        ..
    }) = decode_vobsub(pair.idx.as_bytes(), &pair.sub, super::LIMITS, &mut memory)
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
    timing == [(1000, 1512, true), (1512, 1512, false)]
        && rgba.dimensions() == (8, 4)
        && rgba.get_pixel(4, 1).0 == [255, 255, 255, 255]
        && rgba.get_pixel(2, 2).0 == [255, 0, 0, 136]
        && rgba.get_pixel(2, 1).0 == [0, 0, 0, 0]
}

/// A `.idx`/`.sub` writer laid out the way VobSubRip and mkvextract write a pair; the self-test
/// probe and the tests share it.
pub(crate) mod test_support {
    /// Palette entries: 0 black, 1 white, 2 red, the rest grey.
    const PALETTE: &str = "000000, ffffff, ff0000, 808080, 808080, 808080, 808080, 808080, \
                           808080, 808080, 808080, 808080, 808080, 808080, 808080, 808080";

    pub(crate) struct Pair {
        pub idx: String,
        pub sub: Vec<u8>,
    }

    impl Pair {
        /// An index of one track on a `width`x`height` canvas; `header` lines go before its id.
        pub(crate) fn new((width, height): (u32, u32), header: &str) -> Self {
            Self {
                idx: format!(
                    "# VobSub index file, v7 (do not modify this line!)\nsize: {width}x{height}\n\
                     palette: {PALETTE}\n{header}\nid: en, index: 0\n"
                ),
                sub: Vec::new(),
            }
        }

        /// Indexes `unit` at `ms` and writes it in packs carrying `piece` bytes each.
        pub(crate) fn spu(&mut self, ms: u32, unit: &[u8], piece: usize) {
            let (h, m, s, f) = (ms / 3_600_000, ms / 60_000 % 60, ms / 1000 % 60, ms % 1000);
            self.idx.push_str(&format!(
                "timestamp: {h:02}:{m:02}:{s:02}:{f:03}, filepos: {:09x}\n",
                self.sub.len()
            ));
            for chunk in unit.chunks(piece) {
                self.pack(0x20, chunk);
            }
        }

        /// One pack carrying one private stream 1 packet of `substream`.
        pub(crate) fn pack(&mut self, substream: u8, data: &[u8]) {
            self.sub
                .extend_from_slice(&[0, 0, 1, 0xba, 0x44, 0, 4, 0, 4, 1, 1, 0x89, 0xc3, 0xf8]);
            self.sub.extend_from_slice(&[0, 0, 1, 0xbd]);
            self.sub
                .extend_from_slice(&((data.len() + 4) as u16).to_be_bytes());
            self.sub.extend_from_slice(&[0x81, 0, 0, substream]);
            self.sub.extend_from_slice(data);
        }
    }

    /// One subpicture unit: pattern (1) white opaque, emphasis 1 (2) red at alpha 8, background
    /// (0) and emphasis 2 (3) transparent.
    pub(crate) struct Unit {
        pub x: u32,
        pub y: u32,
        pub width: u32,
        /// Each row's `(length, value)` runs; a zero length runs to the end of the row.
        pub rows: Vec<Vec<(u32, u8)>>,
        pub start: u16,
        pub stop: Option<u16>,
        pub forced: bool,
        pub alpha: [u8; 2],
    }

    impl Unit {
        /// The 4x2 probe at `x`,`y`: row 0 two transparent then two white; row 1 four red,
        /// starting at once and stopping 45 ticks (512 ms) later.
        pub(crate) fn two_rows(x: u32, y: u32) -> Self {
            Self {
                x,
                y,
                width: 4,
                rows: vec![vec![(2, 0), (2, 1)], vec![(4, 2)]],
                start: 0,
                stop: Some(45),
                forced: false,
                alpha: [0x08, 0xf0],
            }
        }

        pub(crate) fn bytes(&self) -> Vec<u8> {
            let field = |parity: usize| {
                let mut nibbles = Vec::new();
                for row in self.rows.iter().skip(parity).step_by(2) {
                    let mut runs = row.clone();
                    // A row that stops short of the width ends with the rest-of-row code.
                    if runs.iter().all(|&(len, _)| len != 0)
                        && runs.iter().map(|&(len, _)| len).sum::<u32>() < self.width
                    {
                        runs.push((0, 0));
                    }
                    for &(len, value) in &runs {
                        let code = len << 2 | u32::from(value);
                        let count = match len {
                            1..=3 => 1,
                            4..=15 => 2,
                            16..=63 => 3,
                            _ => 4,
                        };
                        for shift in (0..count).rev() {
                            nibbles.push((code >> (4 * shift)) as u8 & 0x0f);
                        }
                    }
                    if nibbles.len() % 2 == 1 {
                        nibbles.push(0);
                    }
                }
                nibbles
                    .chunks(2)
                    .map(|pair| pair[0] << 4 | pair[1])
                    .collect::<Vec<u8>>()
            };
            let (top, bottom) = (field(0), field(1));
            let height = self.rows.len() as u32;
            let (x2, y2) = (self.x + self.width - 1, self.y + height - 1);
            let top_at = 4_u16;
            let bottom_at = top_at + top.len() as u16;
            let control = bottom_at + bottom.len() as u16;
            let mut first = vec![if self.forced { 0x00 } else { 0x01 }];
            first.extend_from_slice(&[0x03, 0x02, 0x10, 0x04, self.alpha[0], self.alpha[1], 0x05]);
            first.extend_from_slice(&[
                (self.x >> 4) as u8,
                ((self.x & 0x0f) << 4) as u8 | (x2 >> 8) as u8,
                x2 as u8,
                (self.y >> 4) as u8,
                ((self.y & 0x0f) << 4) as u8 | (y2 >> 8) as u8,
                y2 as u8,
                0x06,
            ]);
            first.extend_from_slice(&top_at.to_be_bytes());
            first.extend_from_slice(&bottom_at.to_be_bytes());
            first.push(0xff);
            let second_at = control + 4 + first.len() as u16;
            let mut control_bytes = self.start.to_be_bytes().to_vec();
            let next = if self.stop.is_some() {
                second_at
            } else {
                control
            };
            control_bytes.extend_from_slice(&next.to_be_bytes());
            control_bytes.extend_from_slice(&first);
            if let Some(stop) = self.stop {
                control_bytes.extend_from_slice(&stop.to_be_bytes());
                control_bytes.extend_from_slice(&second_at.to_be_bytes());
                control_bytes.extend_from_slice(&[0x02, 0xff]);
            }
            let size = control as usize + control_bytes.len();
            let mut unit = (size as u16).to_be_bytes().to_vec();
            unit.extend_from_slice(&control.to_be_bytes());
            unit.extend_from_slice(&top);
            unit.extend_from_slice(&bottom);
            unit.extend_from_slice(&control_bytes);
            unit
        }
    }
}

#[cfg(test)]
mod tests {
    use std::fs;
    use std::path::PathBuf;

    use super::test_support::{Pair, Unit};
    use super::*;
    use crate::hex_digest;
    use crate::subtitles::{LIMITS, Memory, StateResult, decode, run_decode_subtitles};

    const FFMPEG_IDX: &[u8] = include_bytes!("../../testdata/vobsub/track.idx");
    const FFMPEG_SUB: &[u8] = include_bytes!("../../testdata/vobsub/track.sub");

    fn decode_pair(
        idx: &[u8],
        sub: &[u8],
        limits: Limits,
    ) -> Result<(Decoded, Vec<image::RgbaImage>), WorkerError> {
        let mut memory = Memory::default();
        let decoded = decode_vobsub(idx, sub, limits, &mut memory)?;
        let images = memory
            .0
            .iter()
            .map(|png| image::load_from_memory(png).expect("still").to_rgba8())
            .collect();
        Ok((decoded, images))
    }

    fn decode_built(pair: &Pair) -> (Decoded, Vec<image::RgbaImage>) {
        decode_pair(pair.idx.as_bytes(), &pair.sub, LIMITS).expect("decode")
    }

    fn timing(states: &[StateResult]) -> Vec<(i64, i64, bool)> {
        states
            .iter()
            .map(|s| (s.start_ms, s.end_ms, s.still.is_some()))
            .collect()
    }

    fn assert_pixels(img: &image::RgbaImage, want: &[(u32, u32, [u8; 4])]) {
        for &(x, y, colour) in want {
            assert_eq!(img.get_pixel(x, y).0, colour, "pixel ({x},{y})");
        }
    }

    const WHITE: [u8; 4] = [255, 255, 255, 255];
    const RED: [u8; 4] = [255, 0, 0, 136];
    const CLEAR: [u8; 4] = [0, 0, 0, 0];

    /// `testdata/vobsub` holds the pair ffmpeg and mkvextract make from `source.sup` (see its
    /// README): white text rows over black ones, at 1 s and 3 s.
    #[test]
    fn an_ffmpeg_pair_decodes_to_its_states() {
        let (decoded, images) = decode_pair(FFMPEG_IDX, FFMPEG_SUB, LIMITS).expect("decode");
        assert_eq!((decoded.width, decoded.height), (720, 480));
        // ffmpeg codes each stop delay in whole 1024/90000 s ticks, so 1.5 s and 1 s land a
        // few milliseconds early.
        assert_eq!(
            timing(&decoded.states),
            [
                (1000, 2490, true),
                (2490, 3000, false),
                (3000, 3990, true),
                (3990, 3990, false)
            ]
        );
        let black = [0, 0, 0, 255];
        assert_pixels(
            &images[0],
            &[
                (100, 400, WHITE),
                (139, 404, WHITE),
                (100, 405, black),
                (139, 409, black),
                (99, 400, CLEAR),
                (140, 400, CLEAR),
                (100, 410, CLEAR),
            ],
        );
        assert_pixels(
            &images[1],
            &[
                (200, 300, WHITE),
                (263, 303, WHITE),
                (200, 304, black),
                (263, 307, black),
                (100, 400, CLEAR),
                (264, 300, CLEAR),
            ],
        );
    }

    #[test]
    fn fields_interleave_and_pieces_reassemble_across_packs() {
        let unit = Unit {
            x: 1,
            y: 1,
            width: 125,
            rows: vec![
                vec![(3, 1)],
                vec![(3, 2)],
                vec![(0, 1)],
                vec![(20, 1), (5, 0), (100, 2)],
            ],
            ..Unit::two_rows(0, 0)
        }
        .bytes();
        let mut pair = Pair::new((200, 10), "");
        pair.idx
            .push_str("timestamp: 00:00:01:000, filepos: 000000000\n");
        // The unit's pieces sit in separate packs, with another substream and padding between.
        pair.pack(0x20, &unit[..5]);
        pair.pack(0x21, b"another track");
        pair.sub
            .extend_from_slice(&[0, 0, 1, 0xbe, 0, 3, 0xff, 0xff, 0xff]);
        for piece in unit[5..].chunks(7) {
            pair.pack(0x20, piece);
        }
        let (decoded, images) = decode_built(&pair);
        assert_eq!(
            timing(&decoded.states),
            [(1000, 1512, true), (1512, 1512, false)]
        );
        assert_pixels(
            &images[0],
            &[
                (1, 1, WHITE),
                (3, 1, WHITE),
                (4, 1, CLEAR),
                (1, 2, RED),
                (3, 2, RED),
                (4, 2, CLEAR),
                // A zero-length run fills the rest of its row.
                (1, 3, WHITE),
                (125, 3, WHITE),
                (126, 3, CLEAR),
                (1, 4, WHITE),
                (20, 4, WHITE),
                (21, 4, CLEAR),
                (25, 4, CLEAR),
                (26, 4, RED),
                (125, 4, RED),
                (126, 4, CLEAR),
                (0, 0, CLEAR),
            ],
        );
    }

    #[test]
    fn forced_only_shows_only_forced_subpictures() {
        let build = |header: &str| {
            let mut pair = Pair::new((8, 4), header);
            pair.spu(1000, &Unit::two_rows(0, 0).bytes(), 2048);
            let forced = Unit {
                forced: true,
                ..Unit::two_rows(0, 0)
            };
            pair.spu(3000, &forced.bytes(), 2048);
            pair
        };
        let (decoded, _) = decode_built(&build("forced subs: ON"));
        assert_eq!(
            timing(&decoded.states),
            [(3000, 3512, true), (3512, 3512, false)]
        );
        let (decoded, _) = decode_built(&build("forced subs: OFF"));
        assert_eq!(
            timing(&decoded.states),
            [
                (1000, 1512, true),
                (1512, 3000, false),
                (3000, 3512, true),
                (3512, 3512, false)
            ]
        );
    }

    #[test]
    fn empty_subpictures_and_tracks_leave_the_screen_clear() {
        // A subpicture whose every value is transparent shows nothing, so no state is kept.
        let mut pair = Pair::new((8, 4), "");
        let invisible = Unit {
            alpha: [0, 0],
            ..Unit::two_rows(0, 0)
        };
        pair.spu(1000, &invisible.bytes(), 2048);
        let (decoded, images) = decode_built(&pair);
        assert!(decoded.states.is_empty());
        assert!(images.is_empty());
        // A track with no subpictures, as a forced-only extraction often is, keeps its canvas.
        let (decoded, _) = decode_built(&Pair::new((720, 576), ""));
        assert_eq!((decoded.width, decoded.height), (720, 576));
        assert!(decoded.states.is_empty());
    }

    #[test]
    fn a_later_subpicture_replaces_one_still_showing() {
        let mut pair = Pair::new((8, 4), "");
        pair.spu(1000, &Unit::two_rows(0, 0).bytes(), 2048);
        pair.spu(1200, &Unit::two_rows(4, 2).bytes(), 2048);
        let (decoded, images) = decode_built(&pair);
        assert_eq!(
            timing(&decoded.states),
            [(1000, 1200, true), (1200, 1712, true), (1712, 1712, false)]
        );
        assert_pixels(&images[1], &[(2, 0, CLEAR), (6, 2, WHITE)]);

        // Without a stop, each subpicture holds until the next, and the last holds on.
        let held = Unit {
            stop: None,
            ..Unit::two_rows(0, 0)
        };
        let mut pair = Pair::new((8, 4), "");
        pair.spu(1000, &held.bytes(), 2048);
        pair.spu(2000, &held.bytes(), 2048);
        let (decoded, _) = decode_built(&pair);
        assert_eq!(
            timing(&decoded.states),
            [(1000, 2000, true), (2000, 2000, true)]
        );
    }

    #[test]
    fn custom_colours_replace_the_palette() {
        let mut pair = Pair::new(
            (8, 4),
            "custom colors: ON, tridx: 0010, colors: 000000, 00ff00, 0000ff, 000000",
        );
        pair.spu(0, &Unit::two_rows(0, 0).bytes(), 2048);
        let (_, images) = decode_built(&pair);
        // Pattern (1) takes the second custom colour; emphasis 1 (2) is transparent by tridx.
        assert_pixels(&images[0], &[(2, 0, [0, 255, 0, 255]), (0, 1, CLEAR)]);
    }

    #[test]
    fn delays_and_the_time_offset_shift_timestamps() {
        let mut pair = Pair::new((8, 4), "time offset: 500\ndelay: 00:00:01:000");
        pair.spu(1000, &Unit::two_rows(0, 0).bytes(), 2048);
        pair.idx.push_str("delay: -00:00:00:250\n");
        pair.spu(5000, &Unit::two_rows(0, 0).bytes(), 2048);
        let (decoded, _) = decode_built(&pair);
        assert_eq!(
            timing(&decoded.states),
            [
                (2500, 3012, true),
                (3012, 6250, false),
                (6250, 6762, true),
                (6762, 6762, false)
            ]
        );
    }

    /// An SPU of hand-written control bytes after its 4-byte header.
    fn raw_unit(control: &[u8]) -> Vec<u8> {
        let mut unit = ((control.len() + 4) as u16).to_be_bytes().to_vec();
        unit.extend_from_slice(&[0, 4]);
        unit.extend_from_slice(control);
        unit
    }

    #[test]
    fn malformed_and_over_ceiling_pairs_are_refused_with_stable_codes() {
        let probe = Unit::two_rows(0, 0).bytes();
        let one = |header: &str, unit: &[u8]| {
            let mut pair = Pair::new((8, 4), header);
            pair.spu(1000, unit, 2048);
            pair
        };
        let mut no_size = one("", &probe);
        no_size.idx = no_size.idx.replace("size: 8x4\n", "");
        let mut short_palette = one("", &probe);
        short_palette.idx = short_palette.idx.replace("000000, ffffff, ", "ffffff, ");
        let mut two_tracks = one("", &probe);
        two_tracks.idx.push_str("id: fr, index: 1\n");
        let mut nowhere = one("", &probe);
        nowhere
            .idx
            .push_str("timestamp: 00:00:02:000, filepos: 000000010\n");
        let mut truncated = one("", &probe);
        truncated.sub.truncate(truncated.sub.len() - 3);
        let mut garbage = one("", &probe);
        garbage.sub = b"not a program stream".to_vec();
        let mut mpeg1 = one("", &probe);
        mpeg1.sub[4] = 0x21;
        let mut backwards = one("", &probe);
        backwards.spu(500, &probe, 2048);
        let mut bad_time = one("", &probe);
        bad_time
            .idx
            .push_str("timestamp: 00:61:00:000, filepos: 000000000\n");
        let mut two = one("", &probe);
        two.spu(2000, &probe, 2048);
        let wide = Unit {
            width: 101,
            rows: vec![vec![(0, 1)]],
            ..Unit::two_rows(0, 0)
        };
        let mut wide_pair = Pair::new((100, 100), "");
        wide_pair.spu(0, &wide.bytes(), 2048);
        let mut big_canvas = Pair::new((3841, 2160), "");
        big_canvas.spu(0, &probe, 2048);
        // Draws one value-1 run: start, area (0,0)-(3,0), both fields at the trailing zero byte,
        // pattern opaque; the zero byte begins a code the unit ends inside.
        let truncated_run = raw_unit(&[
            0, 0, 0, 4, 0x01, 0x05, 0, 0, 3, 0, 0, 0, 0x06, 0, 25, 0, 25, 0x04, 0, 0xf0, 0xff, 0,
        ]);
        for (name, pair, limits, code) in [
            ("no size", no_size, LIMITS, "corrupt_input"),
            ("short palette", short_palette, LIMITS, "corrupt_input"),
            ("two tracks", two_tracks, LIMITS, "unsupported_input"),
            ("filepos nowhere", nowhere, LIMITS, "corrupt_input"),
            ("truncated sub", truncated, LIMITS, "corrupt_input"),
            ("garbage sub", garbage, LIMITS, "corrupt_input"),
            ("MPEG-1 pack", mpeg1, LIMITS, "corrupt_input"),
            ("backwards", backwards, LIMITS, "corrupt_input"),
            ("bad time", bad_time, LIMITS, "corrupt_input"),
            (
                "unknown command",
                one("", &raw_unit(&[0, 0, 0, 4, 0x09, 0xff])),
                LIMITS,
                "corrupt_input",
            ),
            (
                "unterminated control",
                one("", &raw_unit(&[0, 0, 0, 4, 0x01])),
                LIMITS,
                "corrupt_input",
            ),
            (
                "no display area",
                one("", &raw_unit(&[0, 0, 0, 4, 0x01, 0xff])),
                LIMITS,
                "corrupt_input",
            ),
            (
                "truncated run",
                one("", &truncated_run),
                LIMITS,
                "corrupt_input",
            ),
            ("big canvas", big_canvas, LIMITS, "limit_exceeded"),
            (
                "wide subpicture",
                wide_pair,
                Limits {
                    max_width: 100,
                    ..LIMITS
                },
                "limit_exceeded",
            ),
            (
                "entries",
                two,
                Limits {
                    max_states: 1,
                    ..LIMITS
                },
                "limit_exceeded",
            ),
            (
                "drawn pixels",
                one("", &probe),
                Limits {
                    max_drawn_pixels: 8,
                    ..LIMITS
                },
                "limit_exceeded",
            ),
        ] {
            let err = decode_pair(pair.idx.as_bytes(), &pair.sub, limits)
                .err()
                .unwrap_or_else(|| panic!("{name} decoded"));
            assert_eq!(err.code, code, "{name}: {}", err.message);
        }
    }

    #[test]
    fn the_request_names_the_pair_and_checks_both_files() {
        /// Removes the directory when dropped, so a failing assertion leaves nothing behind.
        struct TempDir(PathBuf);
        impl Drop for TempDir {
            fn drop(&mut self) {
                let _ = fs::remove_dir_all(&self.0);
            }
        }
        let dir =
            TempDir(std::env::temp_dir().join(format!("loomarr-vobsub-{}", std::process::id())));
        let root = &dir.0;
        let staging = root.join("staging");
        fs::create_dir_all(&staging).expect("staging");
        let (idx, sub) = (root.join("track.idx"), root.join("track.sub"));
        fs::write(&idx, FFMPEG_IDX).expect("idx");
        fs::write(&sub, FFMPEG_SUB).expect("sub");
        let request = |codec: &str, index: serde_json::Value| {
            serde_json::json!({
                "protocol": 1, "requestId": "r",
                "source": {
                    "codec": codec, "path": sub, "expectedSha256": hex_digest(FFMPEG_SUB),
                    "index": index,
                },
                "stagingDir": staging,
            })
        };
        let index = serde_json::json!({"path": idx, "expectedSha256": hex_digest(FFMPEG_IDX)});
        let mut out = Vec::new();
        let control = serde_json::to_vec(&request(VOBSUB_CODEC, index.clone())).expect("request");
        run_decode_subtitles(&control[..], &mut out).expect("decode");
        let result: serde_json::Value = serde_json::from_slice(&out).expect("result");
        assert_eq!(result["source"]["codec"], VOBSUB_CODEC);
        assert_eq!(result["source"]["sha256"], hex_digest(FFMPEG_SUB));
        assert_eq!(result["source"]["bytes"], FFMPEG_SUB.len());
        assert_eq!(
            (result["width"].clone(), result["height"].clone()),
            (720.into(), 480.into())
        );
        assert_eq!(result["states"].as_array().map(Vec::len), Some(4));
        let png = fs::read(staging.join("s00000.png")).expect("still");
        assert_eq!(result["states"][0]["still"]["sha256"], hex_digest(&png));

        let stale_index = serde_json::json!({"path": idx, "expectedSha256": hex_digest(b"other")});
        let stale_sub = {
            let mut r = request(VOBSUB_CODEC, index.clone());
            r["source"]["expectedSha256"] = hex_digest(b"other").into();
            r
        };
        let mut unknown_field = request(VOBSUB_CODEC, index.clone());
        unknown_field["source"]["index"]["codec"] = VOBSUB_CODEC.into();
        let small_index = Limits {
            max_index_bytes: 16,
            ..LIMITS
        };
        for (name, request, limits, code) in [
            (
                "no index",
                request(VOBSUB_CODEC, serde_json::Value::Null),
                LIMITS,
                "invalid_request",
            ),
            (
                "PGS with an index",
                request(super::super::PGS_CODEC, index.clone()),
                LIMITS,
                "invalid_request",
            ),
            (
                "stale index",
                request(VOBSUB_CODEC, stale_index),
                LIMITS,
                "source_changed",
            ),
            ("stale sub", stale_sub, LIMITS, "source_changed"),
            (
                "index ceiling",
                request(VOBSUB_CODEC, index.clone()),
                small_index,
                "limit_exceeded",
            ),
        ] {
            let request = serde_json::from_value(request).expect("request");
            let err = decode(request, limits)
                .err()
                .unwrap_or_else(|| panic!("{name} decoded"));
            assert_eq!(err.code, code, "{name}: {}", err.message);
        }
        let control = serde_json::to_vec(&unknown_field).expect("request");
        let err = run_decode_subtitles(&control[..], Vec::new()).expect_err("refused");
        assert_eq!(err.code, "invalid_request");
    }
}
