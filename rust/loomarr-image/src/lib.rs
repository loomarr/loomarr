use std::fs::{self, OpenOptions};
use std::io::{Cursor, Read, Write};
use std::path::{Path, PathBuf};

use base64::Engine as _;
use fast_image_resize::{PixelType, ResizeOptions, Resizer, images::Image as ResizeImage};
use image::{
    AnimationDecoder, ImageEncoder, ImageFormat, ImageReader,
    codecs::{gif::GifDecoder, png::PngDecoder, webp::WebPDecoder},
    metadata::LoopCount,
};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

pub const PROTOCOL: u32 = 1;
pub const RECIPE: &str = "loomarr-rendition-v1";
const MAX_CONTROL_BYTES: u64 = 1 << 20;

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Capabilities<'a> {
    pub protocol: u32,
    pub release: &'a str,
    pub recipe: &'a str,
    pub formats: [&'a str; 3],
    pub animation: bool,
    pub self_test: bool,
}

pub fn capabilities(release: &str) -> Capabilities<'_> {
    Capabilities {
        protocol: PROTOCOL,
        release,
        recipe: RECIPE,
        formats: ["avif", "jpeg", "webp"],
        animation: true,
        self_test: worker_self_test(),
    }
}

fn worker_self_test() -> bool {
    let pixels = [
        0xff, 0x00, 0x00, 0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff,
        0xff,
    ];
    let webp = webp::Encoder::from_rgba(&pixels, 2, 2).encode_simple(true, 80.0);
    let jpeg = encode_jpeg(&pixels, 2, 2);
    let avif = encode_avif(&pixels, 2, 2);
    let animation = (|| {
        let mut encoder = webp_animation::Encoder::new((2, 2)).ok()?;
        encoder.add_frame(&pixels, 0).ok()?;
        let mut second = pixels;
        second[0] = 0;
        encoder.add_frame(&second, 100).ok()?;
        encoder.finalize(200).ok()
    })();
    matches!(webp, Ok(ref bytes) if bytes.starts_with(b"RIFF") && bytes.get(8..12) == Some(b"WEBP"))
        && matches!(jpeg, Ok(ref bytes) if bytes.starts_with(&[0xff, 0xd8]))
        && matches!(avif, Ok(ref bytes) if bytes.get(4..8) == Some(b"ftyp"))
        && matches!(animation, Some(ref bytes) if webp::BitstreamFeatures::new(bytes.as_ref()).is_some_and(|f| f.has_animation()))
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct GenerateRequest {
    protocol: u32,
    request_id: String,
    source: SourceRequest,
    staging_dir: PathBuf,
    targets: Vec<TargetRequest>,
    budget: Budget,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct SourceRequest {
    path: PathBuf,
    expected_sha256: String,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct TargetRequest {
    id: String,
    format: String,
    width: u32,
    motion: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Budget {
    max_input_bytes: u64,
    max_width: u32,
    max_height: u32,
    max_canvas_pixels: u64,
    max_frames: u32,
    max_total_frame_pixels: u64,
    max_duration_ms: u64,
    max_output_bytes: u64,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct GenerateResult {
    protocol: u32,
    status: &'static str,
    request_id: String,
    source: SourceResult,
    outputs: Vec<OutputResult>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct SourceResult {
    sha256: String,
    mime: &'static str,
    width: u32,
    height: u32,
    bytes: u64,
    animated: bool,
    frame_count: u32,
    duration_ms: u64,
    loop_count: Option<u32>,
    placeholder: String,
    dominant_hex: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct OutputResult {
    target_id: String,
    relative_path: String,
    recipe_id: &'static str,
    format: String,
    mime: &'static str,
    requested_width: u32,
    width: u32,
    height: u32,
    bytes: u64,
    sha256: String,
    animated: bool,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct ErrorResult<'a> {
    protocol: u32,
    status: &'static str,
    request_id: &'a str,
    error: ErrorBody<'a>,
}

#[derive(Debug, Serialize)]
struct ErrorBody<'a> {
    code: &'a str,
    message: &'a str,
}

#[derive(Debug)]
pub struct WorkerError {
    pub code: &'static str,
    pub message: String,
}

impl WorkerError {
    fn new(code: &'static str, message: impl Into<String>) -> Self {
        Self {
            code,
            message: message.into(),
        }
    }
}

pub fn run_generate(input: impl Read, output: impl Write) -> Result<(), WorkerError> {
    let mut input = input.take(MAX_CONTROL_BYTES + 1);
    let mut control = Vec::new();
    input
        .read_to_end(&mut control)
        .map_err(|err| WorkerError::new("invalid_request", format!("read request: {err}")))?;
    if control.len() as u64 > MAX_CONTROL_BYTES {
        return Err(WorkerError::new(
            "invalid_request",
            "request exceeds control-frame limit",
        ));
    }
    let request: GenerateRequest = serde_json::from_slice(&control)
        .map_err(|err| WorkerError::new("invalid_request", format!("request JSON: {err}")))?;
    let request_id = request.request_id.clone();
    match generate(request) {
        Ok(result) => {
            serde_json::to_writer(output, &result)
                .map_err(|err| WorkerError::new("internal", format!("result JSON: {err}")))?;
            Ok(())
        }
        Err(err) => {
            serde_json::to_writer(
                output,
                &ErrorResult {
                    protocol: PROTOCOL,
                    status: "error",
                    request_id: &request_id,
                    error: ErrorBody {
                        code: err.code,
                        message: &err.message,
                    },
                },
            )
            .map_err(|json_err| {
                WorkerError::new("internal", format!("error result JSON: {json_err}"))
            })?;
            Err(err)
        }
    }
}

fn generate(request: GenerateRequest) -> Result<GenerateResult, WorkerError> {
    if request.protocol != PROTOCOL {
        return Err(WorkerError::new("invalid_request", "protocol mismatch"));
    }
    if request.targets.len() > 16 {
        return Err(WorkerError::new(
            "limit_exceeded",
            "target count exceeds hard ceiling",
        ));
    }
    let source_meta = fs::symlink_metadata(&request.source.path)
        .map_err(|err| WorkerError::new("io_failed", format!("stat source: {err}")))?;
    if !source_meta.is_file() {
        return Err(WorkerError::new(
            "invalid_request",
            "source is not a regular file",
        ));
    }
    if source_meta.len() > request.budget.max_input_bytes {
        return Err(WorkerError::new(
            "limit_exceeded",
            "source exceeds maxInputBytes",
        ));
    }
    let source = fs::read(&request.source.path)
        .map_err(|err| WorkerError::new("io_failed", format!("read source: {err}")))?;
    let source_hash = hex_digest(&source);
    if source_hash != request.source.expected_sha256 {
        return Err(WorkerError::new(
            "source_changed",
            "source SHA-256 does not match request",
        ));
    }

    let format = image::guess_format(&source)
        .map_err(|_| WorkerError::new("unsupported_input", "unsupported image signature"))?;
    let mime = mime_for(format)?;
    let reader = ImageReader::with_format(Cursor::new(&source), format);
    let (width, height) = reader
        .into_dimensions()
        .map_err(|err| WorkerError::new("corrupt_input", format!("inspect source: {err}")))?;
    enforce_dimensions(width, height, &request.budget)?;

    let animation = match format {
        ImageFormat::Gif => {
            let decoder = GifDecoder::new(Cursor::new(&source))
                .map_err(|err| WorkerError::new("decode_failed", format!("decode GIF: {err}")))?;
            let loop_count = canonical_loop_count(decoder.loop_count());
            generate_animation(
                &request,
                AnimationSource {
                    frames: decoder.into_frames(),
                    loop_count,
                    hash: source_hash.clone(),
                    mime,
                    width,
                    height,
                    bytes: source.len() as u64,
                },
            )?
        }
        ImageFormat::Png => {
            let decoder = PngDecoder::new(Cursor::new(&source))
                .map_err(|err| WorkerError::new("decode_failed", format!("decode PNG: {err}")))?;
            if decoder
                .is_apng()
                .map_err(|err| WorkerError::new("decode_failed", format!("inspect APNG: {err}")))?
            {
                let decoder = decoder.apng().map_err(|err| {
                    WorkerError::new("decode_failed", format!("decode APNG: {err}"))
                })?;
                let loop_count = canonical_loop_count(decoder.loop_count());
                generate_animation(
                    &request,
                    AnimationSource {
                        frames: decoder.into_frames(),
                        loop_count,
                        hash: source_hash.clone(),
                        mime,
                        width,
                        height,
                        bytes: source.len() as u64,
                    },
                )?
            } else {
                None
            }
        }
        ImageFormat::WebP => {
            let decoder = WebPDecoder::new(Cursor::new(&source))
                .map_err(|err| WorkerError::new("decode_failed", format!("decode WebP: {err}")))?;
            if decoder.has_animation() {
                let loop_count = canonical_loop_count(decoder.loop_count());
                generate_animation(
                    &request,
                    AnimationSource {
                        frames: decoder.into_frames(),
                        loop_count,
                        hash: source_hash.clone(),
                        mime,
                        width,
                        height,
                        bytes: source.len() as u64,
                    },
                )?
            } else {
                None
            }
        }
        _ => None,
    };
    if let Some(result) = animation {
        return Ok(result);
    }

    let decoded = image::load_from_memory_with_format(&source, format)
        .map_err(|err| WorkerError::new("decode_failed", format!("decode source: {err}")))?;
    let rgba = decoded.to_rgba8();
    let (placeholder, dominant_hex) = image_metadata(rgba.as_raw(), width, height)?;
    let mut outputs = Vec::with_capacity(request.targets.len());
    let mut output_bytes = 0_u64;

    for target in request.targets {
        validate_target(&target)?;
        let target_width = target.width.min(width);
        let target_height =
            ((u64::from(height) * u64::from(target_width)) / u64::from(width)).max(1) as u32;
        let resized = resize_rgba(rgba.as_raw(), width, height, target_width, target_height)?;
        let (encoded, extension, output_mime) = match target.format.as_str() {
            "webp" => (
                webp::Encoder::from_rgba(&resized, target_width, target_height)
                    .encode_simple(false, 80.0)
                    .map_err(|err| {
                        WorkerError::new("encode_failed", format!("encode WebP: {err:?}"))
                    })?
                    .to_vec(),
                "webp",
                "image/webp",
            ),
            "jpeg" => (
                encode_jpeg(&resized, target_width, target_height)?,
                "jpg",
                "image/jpeg",
            ),
            "avif" => (
                encode_avif(&resized, target_width, target_height)?,
                "avif",
                "image/avif",
            ),
            _ => {
                return Err(WorkerError::new(
                    "unsupported_target",
                    "unsupported static output format",
                ));
            }
        };
        output_bytes = output_bytes.saturating_add(encoded.len() as u64);
        if output_bytes > request.budget.max_output_bytes {
            return Err(WorkerError::new(
                "limit_exceeded",
                "outputs exceed maxOutputBytes",
            ));
        }

        let filename = format!("{}.{}", target.id, extension);
        write_complete(&request.staging_dir, &filename, &encoded)?;
        outputs.push(OutputResult {
            target_id: target.id,
            relative_path: filename,
            recipe_id: RECIPE,
            format: target.format,
            mime: output_mime,
            requested_width: target.width,
            width: target_width,
            height: target_height,
            bytes: encoded.len() as u64,
            sha256: hex_digest(&encoded),
            animated: false,
        });
    }

    Ok(GenerateResult {
        protocol: PROTOCOL,
        status: "ok",
        request_id: request.request_id,
        source: SourceResult {
            sha256: source_hash,
            mime,
            width,
            height,
            bytes: source.len() as u64,
            animated: false,
            frame_count: 1,
            duration_ms: 0,
            loop_count: None,
            placeholder,
            dominant_hex,
        },
        outputs,
    })
}

fn encode_jpeg(pixels: &[u8], width: u32, height: u32) -> Result<Vec<u8>, WorkerError> {
    let mut rgb = Vec::with_capacity((u64::from(width) * u64::from(height) * 3) as usize);
    for pixel in pixels.chunks_exact(4) {
        rgb.extend_from_slice(&pixel[..3]);
    }
    let mut encoded = Vec::new();
    image::codecs::jpeg::JpegEncoder::new_with_quality(&mut encoded, 85)
        .encode(&rgb, width, height, image::ExtendedColorType::Rgb8)
        .map_err(|err| WorkerError::new("encode_failed", format!("encode JPEG: {err}")))?;
    Ok(encoded)
}

fn encode_avif(pixels: &[u8], width: u32, height: u32) -> Result<Vec<u8>, WorkerError> {
    let mut encoded = Vec::new();
    image::codecs::avif::AvifEncoder::new_with_speed_quality(&mut encoded, 6, 70)
        .with_num_threads(Some(1))
        .write_image(pixels, width, height, image::ExtendedColorType::Rgba8)
        .map_err(|err| WorkerError::new("encode_failed", format!("encode AVIF: {err}")))?;
    Ok(encoded)
}

fn image_metadata(pixels: &[u8], width: u32, height: u32) -> Result<(String, String), WorkerError> {
    let (thumb_width, thumb_height) = if width >= height {
        let w = width.min(100);
        (
            w,
            ((u64::from(height) * u64::from(w)) / u64::from(width)).max(1) as u32,
        )
    } else {
        let h = height.min(100);
        (
            ((u64::from(width) * u64::from(h)) / u64::from(height)).max(1) as u32,
            h,
        )
    };
    let thumbnail = resize_rgba(pixels, width, height, thumb_width, thumb_height)?;
    let thumb =
        thumbhash::rgba_to_thumb_hash(thumb_width as usize, thumb_height as usize, &thumbnail);

    let mut red = 0_u64;
    let mut green = 0_u64;
    let mut blue = 0_u64;
    let mut alpha = 0_u64;
    for pixel in pixels.chunks_exact(4) {
        let a = u64::from(pixel[3]);
        red += u64::from(pixel[0]) * a;
        green += u64::from(pixel[1]) * a;
        blue += u64::from(pixel[2]) * a;
        alpha += a;
    }
    let dominant = if alpha == 0 {
        "#000000".to_owned()
    } else {
        format!(
            "#{:02x}{:02x}{:02x}",
            red / alpha,
            green / alpha,
            blue / alpha
        )
    };
    Ok((
        base64::engine::general_purpose::STANDARD.encode(thumb),
        dominant,
    ))
}

struct AnimationTarget {
    request: TargetRequest,
    width: u32,
    height: u32,
    encoder: webp_animation::Encoder,
}

struct AnimationSource<'a> {
    frames: image::Frames<'a>,
    loop_count: u32,
    hash: String,
    mime: &'static str,
    width: u32,
    height: u32,
    bytes: u64,
}

fn generate_animation(
    request: &GenerateRequest,
    mut source: AnimationSource<'_>,
) -> Result<Option<GenerateResult>, WorkerError> {
    let first = source
        .frames
        .next()
        .ok_or_else(|| WorkerError::new("corrupt_input", "animation contains no frames"))?
        .map_err(|err| WorkerError::new("decode_failed", format!("decode frame: {err}")))?;
    let Some(second) = source.frames.next() else {
        return Ok(None);
    };
    let second =
        second.map_err(|err| WorkerError::new("decode_failed", format!("decode frame: {err}")))?;
    let (placeholder, dominant_hex) =
        image_metadata(first.buffer().as_raw(), source.width, source.height)?;

    let mut targets = Vec::with_capacity(request.targets.len());
    let mut outputs = Vec::with_capacity(request.targets.len());
    let mut output_bytes = 0_u64;
    for target in &request.targets {
        validate_target(target)?;
        let target_width = target.width.min(source.width);
        let target_height = ((u64::from(source.height) * u64::from(target_width))
            / u64::from(source.width))
        .max(1) as u32;
        if target.motion == "first_frame" {
            let resized = resize_rgba(
                first.buffer().as_raw(),
                source.width,
                source.height,
                target_width,
                target_height,
            )?;
            let (encoded, extension, output_mime) = match target.format.as_str() {
                "webp" => (
                    webp::Encoder::from_rgba(&resized, target_width, target_height)
                        .encode_simple(false, 80.0)
                        .map_err(|err| {
                            WorkerError::new("encode_failed", format!("encode WebP: {err:?}"))
                        })?
                        .to_vec(),
                    "webp",
                    "image/webp",
                ),
                "jpeg" => (
                    encode_jpeg(&resized, target_width, target_height)?,
                    "jpg",
                    "image/jpeg",
                ),
                "avif" => (
                    encode_avif(&resized, target_width, target_height)?,
                    "avif",
                    "image/avif",
                ),
                _ => {
                    return Err(WorkerError::new(
                        "unsupported_target",
                        "unsupported first-frame output format",
                    ));
                }
            };
            output_bytes = output_bytes.saturating_add(encoded.len() as u64);
            if output_bytes > request.budget.max_output_bytes {
                return Err(WorkerError::new(
                    "limit_exceeded",
                    "outputs exceed maxOutputBytes",
                ));
            }
            let filename = format!("{}.{}", target.id, extension);
            write_complete(&request.staging_dir, &filename, &encoded)?;
            outputs.push(OutputResult {
                target_id: target.id.clone(),
                relative_path: filename,
                recipe_id: RECIPE,
                format: target.format.clone(),
                mime: output_mime,
                requested_width: target.width,
                width: target_width,
                height: target_height,
                bytes: encoded.len() as u64,
                sha256: hex_digest(&encoded),
                animated: false,
            });
            continue;
        }
        if target.format != "webp" {
            return Err(WorkerError::new(
                "unsupported_target",
                "preserving animated input requires WebP output",
            ));
        }
        let mut options = webp_animation::EncoderOptions::default();
        options.anim_params.loop_count = i32::try_from(source.loop_count).map_err(|_| {
            WorkerError::new("limit_exceeded", "loop count exceeds WebP representation")
        })?;
        options.encoding_config = Some(webp_animation::EncodingConfig::new_lossy(80.0));
        let encoder =
            webp_animation::Encoder::new_with_options((target_width, target_height), options)
                .map_err(|err| {
                    WorkerError::new("encode_failed", format!("create animation encoder: {err}"))
                })?;
        targets.push(AnimationTarget {
            request: target.clone(),
            width: target_width,
            height: target_height,
            encoder,
        });
    }

    let canvas = u64::from(source.width) * u64::from(source.height);
    let mut frame_count = 0_u32;
    let mut timestamp = 0_u64;
    let mut elapsed_micros = 0_u64;
    for frame in std::iter::once(Ok(first))
        .chain(std::iter::once(Ok(second)))
        .chain(source.frames)
    {
        let frame = frame
            .map_err(|err| WorkerError::new("decode_failed", format!("decode frame: {err}")))?;
        frame_count = frame_count.saturating_add(1);
        if frame_count > request.budget.max_frames
            || canvas.saturating_mul(u64::from(frame_count)) > request.budget.max_total_frame_pixels
        {
            return Err(WorkerError::new(
                "limit_exceeded",
                "animation frame work exceeds budget",
            ));
        }
        let frame_timestamp = i32::try_from(timestamp)
            .map_err(|_| WorkerError::new("limit_exceeded", "animation timestamp exceeds WebP"))?;
        for target in &mut targets {
            let resized = resize_rgba(
                frame.buffer().as_raw(),
                source.width,
                source.height,
                target.width,
                target.height,
            )?;
            target
                .encoder
                .add_frame(&resized, frame_timestamp)
                .map_err(|err| {
                    WorkerError::new("encode_failed", format!("add animation frame: {err}"))
                })?;
        }
        let delay_micros = canonical_delay_micros(frame.delay());
        elapsed_micros = elapsed_micros.saturating_add(delay_micros);
        // WebP timestamps are whole milliseconds. Round cumulative time instead of every frame
        // independently so a 60fps APNG does not gain roughly a third of a millisecond per frame.
        timestamp = ((elapsed_micros.saturating_add(500)) / 1_000).max(timestamp + 1);
        if timestamp > request.budget.max_duration_ms {
            return Err(WorkerError::new(
                "limit_exceeded",
                "animation duration exceeds budget",
            ));
        }
    }

    let final_timestamp = i32::try_from(timestamp)
        .map_err(|_| WorkerError::new("limit_exceeded", "animation duration exceeds WebP"))?;
    for target in targets {
        let encoded = target.encoder.finalize(final_timestamp).map_err(|err| {
            WorkerError::new("encode_failed", format!("finalize animation: {err}"))
        })?;
        output_bytes = output_bytes.saturating_add(encoded.len() as u64);
        if output_bytes > request.budget.max_output_bytes {
            return Err(WorkerError::new(
                "limit_exceeded",
                "outputs exceed maxOutputBytes",
            ));
        }
        let filename = format!("{}.webp", target.request.id);
        write_complete(&request.staging_dir, &filename, encoded.as_ref())?;
        outputs.push(OutputResult {
            target_id: target.request.id,
            relative_path: filename,
            recipe_id: RECIPE,
            format: target.request.format,
            mime: "image/webp",
            requested_width: target.request.width,
            width: target.width,
            height: target.height,
            bytes: encoded.len() as u64,
            sha256: hex_digest(encoded.as_ref()),
            animated: true,
        });
    }

    outputs.sort_by_key(|output| {
        request
            .targets
            .iter()
            .position(|target| target.id == output.target_id)
            .unwrap_or(usize::MAX)
    });

    Ok(Some(GenerateResult {
        protocol: PROTOCOL,
        status: "ok",
        request_id: request.request_id.clone(),
        source: SourceResult {
            sha256: source.hash,
            mime: source.mime,
            width: source.width,
            height: source.height,
            bytes: source.bytes,
            animated: true,
            frame_count,
            duration_ms: timestamp,
            loop_count: Some(source.loop_count),
            placeholder,
            dominant_hex,
        },
        outputs,
    }))
}

fn canonical_loop_count(loop_count: LoopCount) -> u32 {
    match loop_count {
        LoopCount::Infinite => 0,
        LoopCount::Finite(count) => count.get(),
    }
}

fn canonical_delay_micros(delay: image::Delay) -> u64 {
    let (numerator_ms, denominator) = delay.numer_denom_ms();
    if numerator_ms == 0 || denominator == 0 {
        // GIF/APNG both permit a zero delay and leave the viewer to impose a lower bound. Make
        // that ambiguity explicit and deterministic before converting to WebP.
        return 10_000;
    }
    ((u64::from(numerator_ms) * 1_000 + u64::from(denominator) / 2) / u64::from(denominator))
        .max(1_000)
}

fn enforce_dimensions(width: u32, height: u32, budget: &Budget) -> Result<(), WorkerError> {
    if width == 0 || height == 0 || width > budget.max_width || height > budget.max_height {
        return Err(WorkerError::new(
            "limit_exceeded",
            "source dimensions exceed budget",
        ));
    }
    let canvas = u64::from(width) * u64::from(height);
    if canvas > budget.max_canvas_pixels || canvas > budget.max_total_frame_pixels {
        return Err(WorkerError::new(
            "limit_exceeded",
            "source canvas exceeds budget",
        ));
    }
    if budget.max_frames == 0 || budget.max_duration_ms == 0 {
        return Err(WorkerError::new(
            "limit_exceeded",
            "animation budget cannot be zero",
        ));
    }
    Ok(())
}

fn validate_target(target: &TargetRequest) -> Result<(), WorkerError> {
    if target.id.is_empty()
        || !target
            .id
            .bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
        || target.width == 0
    {
        return Err(WorkerError::new("invalid_request", "invalid target"));
    }
    if target.motion != "preserve" && target.motion != "first_frame" {
        return Err(WorkerError::new("invalid_request", "invalid target motion"));
    }
    Ok(())
}

fn resize_rgba(
    pixels: &[u8],
    width: u32,
    height: u32,
    target_width: u32,
    target_height: u32,
) -> Result<Vec<u8>, WorkerError> {
    if width == target_width && height == target_height {
        return Ok(pixels.to_vec());
    }
    let src = ResizeImage::from_vec_u8(width, height, pixels.to_vec(), PixelType::U8x4)
        .map_err(|err| WorkerError::new("decode_failed", format!("source pixels: {err}")))?;
    let mut dst = ResizeImage::new(target_width, target_height, PixelType::U8x4);
    Resizer::new()
        .resize(&src, &mut dst, &ResizeOptions::new())
        .map_err(|err| WorkerError::new("encode_failed", format!("resize: {err}")))?;
    Ok(dst.into_vec())
}

fn mime_for(format: ImageFormat) -> Result<&'static str, WorkerError> {
    match format {
        ImageFormat::Png => Ok("image/png"),
        ImageFormat::Jpeg => Ok("image/jpeg"),
        ImageFormat::WebP => Ok("image/webp"),
        ImageFormat::Gif => Ok("image/gif"),
        _ => Err(WorkerError::new(
            "unsupported_input",
            "input format is outside the raster allowlist",
        )),
    }
}

fn write_complete(staging: &Path, filename: &str, bytes: &[u8]) -> Result<(), WorkerError> {
    let meta = fs::metadata(staging)
        .map_err(|err| WorkerError::new("io_failed", format!("stat staging: {err}")))?;
    if !meta.is_dir() {
        return Err(WorkerError::new(
            "invalid_request",
            "stagingDir is not a directory",
        ));
    }
    let mut file = OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(staging.join(filename))
        .map_err(|err| WorkerError::new("io_failed", format!("create output: {err}")))?;
    file.write_all(bytes)
        .and_then(|()| file.sync_all())
        .map_err(|err| WorkerError::new("io_failed", format!("write output: {err}")))
}

fn hex_digest(bytes: &[u8]) -> String {
    let digest = Sha256::digest(bytes);
    let mut out = String::with_capacity(digest.len() * 2);
    for byte in digest {
        use std::fmt::Write as _;
        write!(&mut out, "{byte:02x}").expect("write to String");
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::{SystemTime, UNIX_EPOCH};

    #[test]
    fn capabilities_are_the_release_contract() {
        let value = capabilities("test-release");
        assert_eq!(value.protocol, 1);
        assert_eq!(value.release, "test-release");
        assert_eq!(value.recipe, "loomarr-rendition-v1");
        assert_eq!(value.formats, ["avif", "jpeg", "webp"]);
        assert!(value.animation);
        assert!(value.self_test);
    }

    #[test]
    fn apng_timeline_is_inspected_as_animation() {
        let mut bytes = Vec::new();
        {
            let mut encoder = png::Encoder::new(&mut bytes, 4, 2);
            encoder.set_color(png::ColorType::Rgba);
            encoder.set_depth(png::BitDepth::Eight);
            encoder.set_animated(2, 0).expect("animated PNG");
            encoder.set_frame_delay(1, 60).expect("first delay");
            let mut writer = encoder.write_header().expect("APNG header");
            writer
                .write_image_data(&[255, 0, 0, 255].repeat(8))
                .expect("first APNG frame");
            writer.set_frame_delay(1, 60).expect("second delay");
            writer
                .write_image_data(&[0, 0, 255, 255].repeat(8))
                .expect("second APNG frame");
            writer.finish().expect("finish APNG");
        }
        let decoder = PngDecoder::new(Cursor::new(&bytes)).expect("decode APNG fixture");
        assert!(decoder.is_apng().expect("inspect APNG fixture"));
        let decoded_frames = decoder
            .apng()
            .expect("APNG decoder")
            .into_frames()
            .collect_frames()
            .expect("APNG frames");
        assert!(
            decoded_frames.len() >= 2,
            "APNG frames: {}",
            decoded_frames.len()
        );
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("clock")
            .as_nanos();
        let staging = std::env::temp_dir().join(format!("loomarr-apng-{unique}"));
        fs::create_dir(&staging).expect("staging");
        let source = staging.join("source.png");
        fs::write(&source, &bytes).expect("source");
        let request = GenerateRequest {
            protocol: PROTOCOL,
            request_id: "apng".into(),
            source: SourceRequest {
                path: source,
                expected_sha256: hex_digest(&bytes),
            },
            staging_dir: staging.clone(),
            targets: vec![TargetRequest {
                id: "webp-w2".into(),
                format: "webp".into(),
                width: 2,
                motion: "preserve".into(),
            }],
            budget: Budget {
                max_input_bytes: 1 << 20,
                max_width: 100,
                max_height: 100,
                max_canvas_pixels: 10_000,
                max_frames: 10,
                max_total_frame_pixels: 100_000,
                max_duration_ms: 10_000,
                max_output_bytes: 1 << 20,
            },
        };
        let result = generate(request).expect("inspect APNG");
        assert!(result.source.animated);
        assert_eq!(result.source.frame_count, 2);
        assert_eq!(result.source.duration_ms, 33);
        assert_eq!(result.source.loop_count, Some(0));
        assert_eq!(result.outputs.len(), 1);
        assert!(result.outputs[0].animated);
        let rendered = fs::read(staging.join("webp-w2.webp")).expect("rendered APNG WebP");
        assert!(
            webp::BitstreamFeatures::new(&rendered)
                .expect("rendered WebP features")
                .has_animation()
        );
        fs::remove_dir_all(staging).expect("cleanup");
    }

    #[test]
    fn svg_is_refused_by_its_signature() {
        let bytes = br#"<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>"#;
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("clock")
            .as_nanos();
        let staging = std::env::temp_dir().join(format!("loomarr-svg-{unique}"));
        fs::create_dir(&staging).expect("staging");
        let source = staging.join("claimed.png");
        fs::write(&source, bytes).expect("source");
        let request = GenerateRequest {
            protocol: PROTOCOL,
            request_id: "svg".into(),
            source: SourceRequest {
                path: source,
                expected_sha256: hex_digest(bytes),
            },
            staging_dir: staging.clone(),
            targets: vec![],
            budget: Budget {
                max_input_bytes: 1 << 20,
                max_width: 100,
                max_height: 100,
                max_canvas_pixels: 10_000,
                max_frames: 10,
                max_total_frame_pixels: 100_000,
                max_duration_ms: 10_000,
                max_output_bytes: 1 << 20,
            },
        };
        let err = generate(request).expect_err("SVG must be rejected");
        assert_eq!(err.code, "unsupported_input");
        fs::remove_dir_all(staging).expect("cleanup");
    }
}
