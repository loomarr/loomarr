#![no_main]

use std::fs;
use std::io::Cursor;
use std::path::PathBuf;
use std::sync::OnceLock;

use libfuzzer_sys::fuzz_target;
use serde_json::json;
use sha2::{Digest, Sha256};

const GIF_SEED: &[u8] = &[
    b'G', b'I', b'F', b'8', b'9', b'a', 1, 0, 1, 0, 0x80, 0, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0x2c, 0,
    0, 0, 0, 1, 0, 1, 0, 0, 2, 2, 0x44, 1, 0, 0x3b,
];

fn workspace() -> &'static (PathBuf, PathBuf) {
    static PATHS: OnceLock<(PathBuf, PathBuf)> = OnceLock::new();
    PATHS.get_or_init(|| {
        let root = std::env::temp_dir().join(format!("loomarr-image-fuzz-{}", std::process::id()));
        let staging = root.join("staging");
        fs::create_dir_all(&staging).expect("create fuzz staging directory");
        (root.join("source"), staging)
    })
}

fn source_bytes(data: &[u8]) -> Vec<u8> {
    if data.first().is_none_or(|mode| mode & 1 == 0) {
        return data.get(1..).unwrap_or_default().to_vec();
    }
    let mut source = GIF_SEED.to_vec();
    for (index, byte) in data[1..].iter().enumerate() {
        let position = index % source.len();
        source[position] ^= byte;
    }
    source
}

fuzz_target!(|data: &[u8]| {
    let source = source_bytes(data);
    let (source_path, staging_dir) = workspace();
    fs::write(source_path, &source).expect("write fuzz source");
    let digest = format!("{:x}", Sha256::digest(&source));
    let request = json!({
        "protocol": 1,
        "requestId": "fuzz",
        "source": {"path": source_path, "expectedSha256": digest},
        "stagingDir": staging_dir,
        "targets": [],
        "budget": {
            "maxInputBytes": 1_048_576,
            "maxWidth": 4096,
            "maxHeight": 4096,
            "maxCanvasPixels": 4_000_000,
            "maxFrames": 32,
            "maxTotalFramePixels": 64_000_000,
            "maxDurationMs": 60_000,
            "maxOutputBytes": 1_048_576
        }
    });
    let control = serde_json::to_vec(&request).expect("serialize fuzz request");
    let _ = loomarr_image::run_generate(Cursor::new(control), Vec::new());
});
