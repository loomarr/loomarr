use std::fs;
use std::io::Write;
use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::time::{SystemTime, UNIX_EPOCH};

use base64::Engine as _;
use sha2::{Digest, Sha256};

#[test]
fn static_png_generates_a_bounded_webp_rendition() {
    let source = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../dash-before.png");
    let staging = std::env::temp_dir().join(format!(
        "loomarr-image-static-{}-{}",
        std::process::id(),
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("clock after epoch")
            .as_nanos()
    ));
    fs::create_dir(&staging).expect("create staging");

    let request = serde_json::json!({
        "protocol": 1,
        "requestId": "static-fixture",
        "source": {
            "path": source,
            "expectedSha256": "352d4793825be40adcda0080c9d7926a260fabdb08b6e30784888b8c01da1da9"
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
    assert_eq!(result["source"]["width"], 1969);
    assert_eq!(result["source"]["height"], 1160);
    assert_eq!(result["source"]["animated"], false);
    assert_eq!(result["outputs"][0]["targetId"], "webp-w320");
    assert_eq!(result["outputs"][0]["width"], 320);
    assert_eq!(result["outputs"][0]["height"], 188);
    assert_eq!(result["outputs"][1]["targetId"], "jpeg-w320");
    assert_eq!(result["outputs"][2]["targetId"], "avif-w320");

    let rendition = fs::read(staging.join("webp-w320.webp")).expect("read rendition");
    assert_eq!(&rendition[..4], b"RIFF");
    assert_eq!(&rendition[8..12], b"WEBP");
    let jpeg = fs::read(staging.join("jpeg-w320.jpg")).expect("read JPEG rendition");
    assert_eq!(&jpeg[..2], &[0xff, 0xd8]);
    let avif = fs::read(staging.join("avif-w320.avif")).expect("read AVIF rendition");
    assert_eq!(&avif[4..8], b"ftyp");
    fs::remove_dir_all(&staging).expect("remove staging");
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
    let source = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../../dash-before.png");
    let unique = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("clock after epoch")
        .as_nanos();
    let root = std::env::temp_dir().join(format!(
        "loomarr-image-stepped-{}-{unique}",
        std::process::id()
    ));
    let ladder_staging = root.join("ladder");
    let direct_staging = root.join("direct");
    fs::create_dir_all(&ladder_staging).expect("create ladder staging");
    fs::create_dir_all(&direct_staging).expect("create direct staging");

    let render = |staging: &PathBuf, targets: serde_json::Value| {
        let request = serde_json::json!({
            "protocol": 1,
            "requestId": "stepped-fixture",
            "source": {
                "path": source,
                "expectedSha256": "352d4793825be40adcda0080c9d7926a260fabdb08b6e30784888b8c01da1da9"
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
