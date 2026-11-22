#![no_main]

use libfuzzer_sys::fuzz_target;

// The PGS decoder over arbitrary bytes. With the low bit of the first byte set, the input is
// mutated into a valid display set first, so the fuzzer starts past the segment framing.
const PGS_SEED: &[u8] = &[
    b'P', b'G', 0, 0, 0, 0, 0, 0, 0, 0, 0x16, 0, 19, 0, 8, 0, 4, 0x10, 0, 1, 0x80, 0, 0, 1, 0, 1,
    0, 0, 0, 2, 0, 1, b'P', b'G', 0, 0, 0, 0, 0, 0, 0, 0, 0x14, 0, 12, 0, 0, 1, 235, 128, 128, 255,
    2, 16, 128, 128, 128, b'P', b'G', 0, 0, 0, 0, 0, 0, 0, 0, 0x15, 0, 22, 0, 1, 0, 0xc0, 0, 0, 0,
    0, 4, 0, 2, 0, 0x02, 1, 1, 0, 0, 0, 0x84, 2, 0, 0, b'P', b'G', 0, 0, 0, 0, 0, 0, 0, 0, 0x80, 0,
    0,
];

fuzz_target!(|data: &[u8]| {
    let bytes = if data.first().is_some_and(|mode| mode & 1 == 1) {
        let mut seeded = PGS_SEED.to_vec();
        for (index, byte) in data[1..].iter().enumerate() {
            seeded[index % PGS_SEED.len()] ^= byte;
        }
        seeded
    } else {
        data.get(1..).unwrap_or_default().to_vec()
    };
    let _ = loomarr_image::fuzz_decode_pgs(&bytes);
});
