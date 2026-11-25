#![no_main]

use std::sync::OnceLock;

use libfuzzer_sys::fuzz_target;

// Both subtitle decoders over arbitrary bytes. Bit 1 of the first byte picks VOBSUB over PGS.
// With bit 0 set, the input is mutated into a valid track first, so the fuzzer starts past the
// framing: a PGS display set, or the VOBSUB probe's `.sub` under its unchanged `.idx`. Without it,
// PGS takes the bytes as they are and VOBSUB splits them into an `.idx` of the second byte's
// length and a `.sub` of the rest.
const PGS_SEED: &[u8] = &[
    b'P', b'G', 0, 0, 0, 0, 0, 0, 0, 0, 0x16, 0, 19, 0, 8, 0, 4, 0x10, 0, 1, 0x80, 0, 0, 1, 0, 1,
    0, 0, 0, 2, 0, 1, b'P', b'G', 0, 0, 0, 0, 0, 0, 0, 0, 0x14, 0, 12, 0, 0, 1, 235, 128, 128, 255,
    2, 16, 128, 128, 128, b'P', b'G', 0, 0, 0, 0, 0, 0, 0, 0, 0x15, 0, 22, 0, 1, 0, 0xc0, 0, 0, 0,
    0, 4, 0, 2, 0, 0x02, 1, 1, 0, 0, 0, 0x84, 2, 0, 0, b'P', b'G', 0, 0, 0, 0, 0, 0, 0, 0, 0x80, 0,
    0,
];

fn vobsub_seed() -> &'static (Vec<u8>, Vec<u8>) {
    static SEED: OnceLock<(Vec<u8>, Vec<u8>)> = OnceLock::new();
    SEED.get_or_init(loomarr_image::fuzz_vobsub_seed)
}

fn mutate(seed: &[u8], data: &[u8]) -> Vec<u8> {
    let mut seeded = seed.to_vec();
    for (index, byte) in data.iter().enumerate() {
        seeded[index % seed.len()] ^= byte;
    }
    seeded
}

fuzz_target!(|data: &[u8]| {
    let Some((&mode, rest)) = data.split_first() else {
        return;
    };
    let seeded = mode & 1 == 1;
    if mode & 2 == 0 {
        let bytes = if seeded {
            mutate(PGS_SEED, rest)
        } else {
            rest.to_vec()
        };
        let _ = loomarr_image::fuzz_decode_pgs(&bytes);
        return;
    }
    if seeded {
        let (idx, sub) = vobsub_seed();
        let _ = loomarr_image::fuzz_decode_vobsub(idx, &mutate(sub, rest));
        return;
    }
    let Some((&split, rest)) = rest.split_first() else {
        return;
    };
    let (idx, sub) = rest.split_at(usize::from(split).min(rest.len()));
    let _ = loomarr_image::fuzz_decode_vobsub(idx, sub);
});
