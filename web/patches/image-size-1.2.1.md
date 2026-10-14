# image-size 1.2.1 parser correction

Tracking: [#1162](https://github.com/loomarr/loomarr/issues/1162).

The compatible Metro dependency remains exactly 1.2.1. pnpm applies the adjacent patch and binds its
hash into the lockfile. This is a Loomarr-maintained source correction; no upstream patched release
is claimed. GitHub's [ICNS advisory](https://github.com/advisories/GHSA-w3rx-r6r6-pgpr) and
[JXL/HEIF advisory](https://github.com/advisories/GHSA-5p2g-fcmc-qvqq) list affected versions through
2.0.2 and no patched version at the 2026-09-09 review.

Metro 0.87.0 passes permitted image content to image-size, which detects the parser from bytes.
A `.png` extension cannot exclude an ICNS/JXL payload. Bounded child-process reproductions of
zero-length ICNS entries and a zero-length JXL partial-codestream box both aborted the unpatched
consumer; the patched consumer throws a parse error. The tested dimensionless HEIF input already
threw before the patch, and no separate HEIF failure reproduction is claimed.

The patch requires an ICNS entry header and an advancing entry length within the declared file.
It preserves prefix-based dimension extraction when a valid payload extends beyond the read buffer.
The shared container-box reader requires a complete header and a valid advancing size; the legal
ISO BMFF size-zero terminal box is interpreted as the remaining input length. Consequently a caller
finding a matching size-zero box cannot repeatedly revisit its own offset.

The regression command is part of the existing `imports:check` script-test gate:

```sh
node --test scripts/image-size-consumer.test.mjs
```

Tests cover the two app-resolved Metro consumers, the locked Metro 0.87.0 consumer, malformed bytes
disguised as PNG, normal application PNGs, and valid ICNS/HEIF dimension extraction. Each parser
runs in a child with a heap bound and parent timeout, so reverting the patch fails without hanging
the test runner. The lock's exact Metro pin is intentionally explicit in this regression; update
that consumer reference when reviewing a Metro replacement.

Do not dismiss the advisories based only on this patch or describe build-tool exposure as a
demonstrated deployed-server vulnerability. A future compatible upstream correction must pass the
same consumer and native bundle gates before this patch is removed.
