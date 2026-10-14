use std::fs;
use std::io::Write;
use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::time::{SystemTime, UNIX_EPOCH};

use base64::Engine as _;
use sha2::{Digest, Sha256};

const STATIC_WIDTH: u32 = 640;
const STATIC_HEIGHT: u32 = 360;

fn static_png_fixture(label: &str) -> (PathBuf, PathBuf, String) {
    let unique = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("clock after epoch")
        .as_nanos();
    let root = std::env::temp_dir().join(format!(
        "loomarr-image-{label}-{}-{unique}",
        std::process::id()
    ));
    fs::create_dir_all(&root).expect("create fixture root");
    let source = root.join("source.png");

    let mut pixels = Vec::with_capacity((STATIC_WIDTH * STATIC_HEIGHT * 3) as usize);
    for y in 0..STATIC_HEIGHT {
        for x in 0..STATIC_WIDTH {
            pixels.extend_from_slice(&[
                (x % 256) as u8,
                (y % 256) as u8,
                ((x * 3 + y * 5) % 256) as u8,
            ]);
        }
    }

    let file = fs::File::create(&source).expect("create PNG fixture");
    let mut encoder = png::Encoder::new(file, STATIC_WIDTH, STATIC_HEIGHT);
    encoder.set_color(png::ColorType::Rgb);
    encoder.set_depth(png::BitDepth::Eight);
    let mut writer = encoder.write_header().expect("write PNG header");
    writer.write_image_data(&pixels).expect("write PNG pixels");
    drop(writer);

    let source_hash = format!(
        "{:x}",
        Sha256::digest(fs::read(&source).expect("read PNG fixture"))
    );
    (root, source, source_hash)
}

#[test]
fn static_png_generates_a_bounded_webp_rendition() {
    let (root, source, source_hash) = static_png_fixture("static");
    let staging = root.join("staging");
    fs::create_dir(&staging).expect("create staging");

    let request = serde_json::json!({
        "protocol": 1,
        "requestId": "static-fixture",
        "source": {
            "path": source,
            "expectedSha256": source_hash
        },
        "stagingDir": staging,
        "targets": [
            {"id": "webp-w320", "format": "webp", "width": 320, "motion": "preserve"},
            {"id": "jpeg-w320", "format": "jpeg", "width": 320, "motion": "first_frame"},
            {"id": "avif-w320", "format": "avif", "width": 320, "motion": "first_frame"}
        ],
        "budget": {
            "maxInputBytes": 8388608,
            "maxWidth": 16384,
            "maxHeight": 16384,
            "maxCanvasPixels": 40000000,
            "maxFrames": 600,
            "maxTotalFramePixels": 600000000,
            "maxDurationMs": 60000,
            "maxOutputBytes": 67108864
        }
    });

    let mut child = Command::new(env!("CARGO_BIN_EXE_loomarr-image"))
        .args(["generate", "--protocol", "1"])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("start worker");
    serde_json::to_writer(child.stdin.as_mut().expect("worker stdin"), &request)
        .expect("write request");
    child.stdin.take().expect("worker stdin").flush().unwrap();
    let output = child.wait_with_output().expect("wait for worker");
    assert!(
        output.status.success(),
        "worker failed: {}",
        String::from_utf8_lossy(&output.stderr)
    );

    let result: serde_json::Value = serde_json::from_slice(&output.stdout).expect("result JSON");
    assert_eq!(result["status"], "ok");
    assert_eq!(result["source"]["mime"], "image/png");
    assert_eq!(result["source"]["width"], STATIC_WIDTH);
    assert_eq!(result["source"]["height"], STATIC_HEIGHT);
    assert_eq!(result["source"]["animated"], false);
    assert_eq!(result["outputs"][0]["targetId"], "webp-w320");
    assert_eq!(result["outputs"][0]["width"], 320);
    assert_eq!(result["outputs"][0]["height"], 180);
    assert_eq!(result["outputs"][1]["targetId"], "jpeg-w320");
    assert_eq!(result["outputs"][2]["targetId"], "avif-w320");

    let rendition = fs::read(staging.join("webp-w320.webp")).expect("read rendition");
    assert_eq!(&rendition[..4], b"RIFF");
    assert_eq!(&rendition[8..12], b"WEBP");
    let jpeg = fs::read(staging.join("jpeg-w320.jpg")).expect("read JPEG rendition");
    assert_eq!(&jpeg[..2], &[0xff, 0xd8]);
    let avif = fs::read(staging.join("avif-w320.avif")).expect("read AVIF rendition");
    assert_eq!(&avif[4..8], b"ftyp");
    fs::remove_dir_all(&root).expect("remove fixture root");
}

#[test]
fn animated_gif_preserves_its_timeline_in_resized_webp() {
    let source_bytes = base64::engine::general_purpose::STANDARD
        .decode(
            include_str!("../../../internal/testkit/fixtures/images/two-frame-red-blue.gif.b64")
                .trim(),
        )
        .expect("decode fixture");
    let unique = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("clock after epoch")
        .as_nanos();
    let root = std::env::temp_dir().join(format!(
        "loomarr-image-animation-{}-{unique}",
        std::process::id()
    ));
    let staging = root.join("staging");
    fs::create_dir_all(&staging).expect("create staging");
    let source = root.join("source.gif");
    fs::write(&source, &source_bytes).expect("write fixture");
    let source_hash = format!("{:x}", Sha256::digest(&source_bytes));

    let request = serde_json::json!({
        "protocol": 1,
        "requestId": "animation-fixture",
        "source": {"path": source, "expectedSha256": source_hash},
        "stagingDir": staging,
        "targets": [
            {"id": "webp-w2", "format": "webp", "width": 2, "motion": "preserve"},
            {"id": "jpeg-w2", "format": "jpeg", "width": 2, "motion": "first_frame"}
        ],
        "budget": {
            "maxInputBytes": 8388608,
            "maxWidth": 16384,
            "maxHeight": 16384,
            "maxCanvasPixels": 40000000,
            "maxFrames": 600,
            "maxTotalFramePixels": 600000000,
            "maxDurationMs": 60000,
            "maxOutputBytes": 67108864
        }
    });

    let mut child = Command::new(env!("CARGO_BIN_EXE_loomarr-image"))
        .args(["generate", "--protocol", "1"])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("start worker");
    serde_json::to_writer(child.stdin.as_mut().expect("worker stdin"), &request)
        .expect("write request");
    child.stdin.take();
    let output = child.wait_with_output().expect("wait for worker");
    assert!(
        output.status.success(),
        "worker failed: {}",
        String::from_utf8_lossy(&output.stderr)
    );

    let result: serde_json::Value = serde_json::from_slice(&output.stdout).expect("result JSON");
    assert_eq!(result["source"]["mime"], "image/gif");
    assert_eq!(result["source"]["animated"], true);
    assert_eq!(result["source"]["frameCount"], 2);
    assert_eq!(result["source"]["durationMs"], 200);
    assert_eq!(result["source"]["loopCount"], 0);
    assert_eq!(result["outputs"][0]["animated"], true);
    assert_eq!(result["outputs"][1]["targetId"], "jpeg-w2");
    assert_eq!(result["outputs"][1]["animated"], false);

    let rendition = fs::read(staging.join("webp-w2.webp")).expect("read rendition");
    let features = webp::BitstreamFeatures::new(&rendition).expect("WebP features");
    assert!(features.has_animation());
    let jpeg = fs::read(staging.join("jpeg-w2.jpg")).expect("read first-frame JPEG");
    assert_eq!(&jpeg[..2], &[0xff, 0xd8]);
    fs::remove_dir_all(&root).expect("remove fixture root");
}

#[test]
fn static_ladder_steps_down_from_the_preceding_rung() {
    let (root, source, source_hash) = static_png_fixture("stepped");
    let ladder_staging = root.join("ladder");
    let direct_staging = root.join("direct");
    fs::create_dir_all(&ladder_staging).expect("create ladder staging");
    fs::create_dir_all(&direct_staging).expect("create direct staging");

    let render = |staging: &PathBuf, targets: serde_json::Value| {
        let request = serde_json::json!({
            "protocol": 1,
            "requestId": "stepped-fixture",
            "source": {
                "path": &source,
                "expectedSha256": &source_hash
            },
            "stagingDir": staging,
            "targets": targets,
            "budget": {
                "maxInputBytes": 8388608,
                "maxWidth": 16384,
                "maxHeight": 16384,
                "maxCanvasPixels": 40000000,
                "maxFrames": 600,
                "maxTotalFramePixels": 600000000,
                "maxDurationMs": 60000,
                "maxOutputBytes": 67108864
            }
        });
        let mut child = Command::new(env!("CARGO_BIN_EXE_loomarr-image"))
            .args(["generate", "--protocol", "1"])
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .spawn()
            .expect("start worker");
        serde_json::to_writer(child.stdin.as_mut().expect("worker stdin"), &request)
            .expect("write request");
        child.stdin.take();
        let output = child.wait_with_output().expect("wait for worker");
        assert!(
            output.status.success(),
            "worker failed: {}",
            String::from_utf8_lossy(&output.stderr)
        );
        serde_json::from_slice::<serde_json::Value>(&output.stdout).expect("result JSON")
    };

    let ladder = render(
        &ladder_staging,
        serde_json::json!([
            {"id": "jpeg-w185", "format": "jpeg", "width": 185, "motion": "first_frame"},
            {"id": "jpeg-w500", "format": "jpeg", "width": 500, "motion": "first_frame"}
        ]),
    );
    let _direct = render(
        &direct_staging,
        serde_json::json!([
            {"id": "jpeg-w185", "format": "jpeg", "width": 185, "motion": "first_frame"}
        ]),
    );
    assert_eq!(ladder["outputs"][0]["recipeId"], "loomarr-rendition-v2");
    assert_ne!(
        fs::read(ladder_staging.join("jpeg-w185.jpg")).expect("read stepped rendition"),
        fs::read(direct_staging.join("jpeg-w185.jpg")).expect("read direct rendition"),
        "a multi-rung ladder resized the small rung from the full-resolution source"
    );
    fs::remove_dir_all(&root).expect("remove fixture root");
}
