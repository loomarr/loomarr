# FINDINGS — beta binary redistribution evidence (2026-08-16)

This note records engineering evidence for the Linux container artifacts proposed in
[PR #402](https://github.com/mantonx/loomarr/pull/402): BtbN's static GPL FFmpeg build and
yt-dlp's PyInstaller standalone executables. A Docker image run through Docker Desktop on macOS is
still a Linux image; this review does not cover a native macOS application bundle.

This is not legal advice. The sections labelled **Fact** report what the cited primary sources say
or contain. Sections labelled **Release interpretation** are a conservative engineering reading
for a beta release, not a legal conclusion. Questions that need qualified counsel are kept
separate.

## Bottom line

The proposed image is not ready to call redistribution-complete yet. Hashing the executable files
is necessary but does not close source and notice obligations for either standalone artifact:

- BtbN's `n8.1-latest` name is mutable, and its binary archive contains FFmpeg's GPL text but not a
  release-specific bundle of the FFmpeg source, BtbN build machinery, patches, and all statically
  linked dependency source.
- yt-dlp's two PyInstaller executables are combined GPLv3-or-later works according to yt-dlp. The
  yt-dlp source tarball alone is explicitly not the source for the bundled Python and dependency
  stack. The tag's third-party notice file also received a large corrective update two days after
  the release, and the tag's PyInstaller command did not embed that notice file.

An implementable closure is to freeze exact dated binary inputs, create architecture-specific
corresponding-source bundles, publish them beside the beta release at no charge, put exact source
directions and license texts in the image, and retain a signed/hash-addressed manifest tying every
image digest to those files. Keep the release blocked until that evidence exists and counsel has
answered the questions at the end of this note.

## What the controlling license text says

**Fact.** GPLv3 section 1 defines Corresponding Source as all source needed to generate, install,
run, and modify the object code, including scripts controlling those activities. It excludes
general-purpose tools and qualifying System Libraries, but includes specifically designed linked
components. Section 6(d) permits object-code distribution from a designated network place when
equivalent access to Corresponding Source is offered from the same place at no further charge; it
also permits clear directions beside the object code to an equivalent source location. Section
6(b)'s three-year written-offer option is written for object code conveyed in or with a physical
product. The license must accompany covered copies. See the
[GPLv3 license text](https://www.gnu.org/licenses/gpl-3.0.html).

**Fact.** The GNU GPL FAQ says source offered over a network must correspond exactly to the binary
being distributed and should be as easy to obtain as the object code. See
[GNU's GPL FAQ](https://www.gnu.org/licenses/gpl-faq.html#SourceAndBinaryOnDifferentSites).

**Release interpretation.** For an OCI image distributed through GHCR, the least ambiguous
engineering path is GPLv3 section 6(d): publish exact source archives as unauthenticated,
no-charge assets of the same Loomarr release and put stable, human-readable directions beside the
image and inside it. Do not use a written offer as the primary network-distribution mechanism
without counsel's approval.

## BtbN FFmpeg build

### Facts established from upstream

- BtbN describes `latest` as a floating release. Its workflow deletes the `latest` release and tag,
  repacks the dated artifacts, and recreates it. BtbN retains the last 14 daily builds and the last
  build of each month for two years. See the
  [BtbN README](https://github.com/BtbN/FFmpeg-Builds/tree/590a6612d7d961e9258429e501619e0b7d7cbedf#release-retention-policy)
  and the
  [release workflow](https://github.com/BtbN/FFmpeg-Builds/blob/590a6612d7d961e9258429e501619e0b7d7cbedf/.github/workflows/build.yml).
- The `gpl` variant selects `--enable-gpl --enable-version3` and copies `COPYING.GPLv3` into the
  archive. The `8.1` add-in selects the moving `release/8.1` branch, and `build.sh` clones that
  branch at build time. See
  [`defaults-gpl.sh`](https://github.com/BtbN/FFmpeg-Builds/blob/590a6612d7d961e9258429e501619e0b7d7cbedf/variants/defaults-gpl.sh),
  [`8.1.sh`](https://github.com/BtbN/FFmpeg-Builds/blob/590a6612d7d961e9258429e501619e0b7d7cbedf/addins/8.1.sh),
  and
  [`build.sh`](https://github.com/BtbN/FFmpeg-Builds/blob/590a6612d7d961e9258429e501619e0b7d7cbedf/build.sh).
- FFmpeg says `--enable-gpl` changes the resulting FFmpeg license to GPLv2-or-later, and
  `--enable-version3` upgrades it to GPLv3-or-later terms. FFmpeg also identifies external GPL
  libraries and special notice requirements in parts of its tree. See FFmpeg's
  [license overview](https://github.com/FFmpeg/FFmpeg/blob/7c533d0f86f13a06ec93968f6194349665b3536a/LICENSE.md)
  and
  [`configure`](https://github.com/FFmpeg/FFmpeg/blob/7c533d0f86f13a06ec93968f6194349665b3536a/configure).
- FFmpeg's own compliance checklist calls for the exact corresponding source, changes, build and
  configuration instructions, a source archive on the same web server, and a nearby source link.
  It explicitly calls out external libraries. See
  [FFmpeg Legal](https://ffmpeg.org/legal.html).
- BtbN says each entry under `scripts.d` represents an included dependency and that Linux arm64
  omits some dependencies supported by amd64. At the examined build commit, dependency recipes
  normally identify a repository and commit; for example, x264 is fixed to commit
  `0480cb05fa188d37ae87e8f4fd8f1aea3711f7ee`. `download.sh` materializes cached source archives,
  but the release workflow uploads that cache as a workflow artifact, not as a public release
  asset. See the
  [package description](https://github.com/BtbN/FFmpeg-Builds/tree/590a6612d7d961e9258429e501619e0b7d7cbedf#package-list),
  [x264 recipe](https://github.com/BtbN/FFmpeg-Builds/blob/590a6612d7d961e9258429e501619e0b7d7cbedf/scripts.d/50-x264.sh),
  [`download.sh`](https://github.com/BtbN/FFmpeg-Builds/blob/590a6612d7d961e9258429e501619e0b7d7cbedf/download.sh),
  and the
  [release workflow](https://github.com/BtbN/FFmpeg-Builds/blob/590a6612d7d961e9258429e501619e0b7d7cbedf/.github/workflows/build.yml).
- BtbN's build machinery is MIT-licensed. Its binary archive's FFmpeg GPL file is not a substitute
  for notices and source for every enabled static dependency. See BtbN's
  [MIT license](https://github.com/BtbN/FFmpeg-Builds/blob/590a6612d7d961e9258429e501619e0b7d7cbedf/LICENSE).

### Candidate immutable binary snapshot

This is a research-date candidate, not an approved Dockerfile pin. It must still pass Loomarr's
unchanged FFmpeg test suite on both architectures.

| Evidence | amd64 | arm64 |
| --- | --- | --- |
| BtbN dated release | `autobuild-2026-08-16-13-00` | same |
| BtbN build commit | `590a6612d7d961e9258429e501619e0b7d7cbedf` | same |
| FFmpeg commit | `7c533d0f86f13a06ec93968f6194349665b3536a` | same |
| Asset | `ffmpeg-n8.1.2-44-g7c533d0f86-linux64-gpl-8.1.tar.xz` | `ffmpeg-n8.1.2-44-g7c533d0f86-linuxarm64-gpl-8.1.tar.xz` |
| SHA-256 | `17780994c4679806fb227676f66a0af30c6379afc770324829f48f2a379be558` | `e970a7dd450b440a21126a8bac3a1c95178b6ba05bee2465a4d2a586345c81ac` |

The filenames and digests come from the dated release's GitHub asset metadata, and the FFmpeg
identifier resolves to the
[full FFmpeg commit](https://github.com/FFmpeg/FFmpeg/commit/7c533d0f86f13a06ec93968f6194349665b3536a).
See the
[dated BtbN release](https://github.com/BtbN/FFmpeg-Builds/releases/tag/autobuild-2026-08-16-13-00)
and its
[release API record](https://api.github.com/repos/BtbN/FFmpeg-Builds/releases/tags/autobuild-2026-08-16-13-00).

### Release interpretation and required evidence

For each architecture, retain and publish:

1. The exact BtbN binary archive, upstream checksum file, and a Loomarr SHA-256 manifest.
2. BtbN source at commit `590a6612d7d961e9258429e501619e0b7d7cbedf`, including its MIT license,
   `scripts.d`, variants, add-ins, Dockerfiles, patches, generation scripts, and build scripts.
3. FFmpeg source at commit `7c533d0f86f13a06ec93968f6194349665b3536a`, including all license files.
4. Every source archive consumed by `download.sh` for that target/variant/add-in. Generate a
   machine-readable inventory with stage name, project, repository URL, immutable revision,
   downloaded-archive SHA-256, license identifier, license-file path, and whether the stage is
   enabled on that architecture. Preserve the generated build configuration and any patches.
5. Captured output from `ffmpeg -version`, `ffmpeg -buildconf`, `ffprobe -version`, and `ldd` for the
   shipped files. Include an empty `changes.diff` or an explicit no-downstream-changes record when
   that is true; otherwise include the actual diff.

The BtbN source-code archive alone is only the build recipes. The FFmpeg source archive alone omits
the statically linked dependency sources. Neither is a complete release evidence set by itself.

## yt-dlp PyInstaller executables

### Facts established from upstream

- yt-dlp says its source repository, source tarball, and wheel are under the Unlicense, while its
  PyInstaller executables include GPLv3-or-later code and are distributed as combined works under
  GPLv3-or-later. It says the standalone builds include Python and packages marked in its
  dependency list. See the tag-specific
  [README](https://github.com/yt-dlp/yt-dlp/blob/2026.07.04/README.md#license).
- The tag has a 226 KB `THIRD_PARTY_LICENSES.txt` aggregating license texts for the bundled
  components and offering source from upstream maintainers. See the tag-specific
  [third-party license file](https://github.com/yt-dlp/yt-dlp/blob/2026.07.04/THIRD_PARTY_LICENSES.txt).
- The release is commit `fdec00e0bf530dc6c3cc7b1dd780e95d9ae460e9`. The amd64 and arm64 Linux
  PyInstaller assets and their GitHub-reported SHA-256 digests match PR #402:

| Architecture | Asset | SHA-256 |
| --- | --- | --- |
| amd64 | `yt-dlp_linux` | `6bbb3d314cde4febe36e5fa1d55462e29c974f63444e707871834f6d8cc210ae` |
| arm64 | `yt-dlp_linux_aarch64` | `b6ce97646773070d7a7ffd6bbbdcaecb47c48483909c54c915bf08a7a9b5e0b1` |

  See the
  [2026.07.04 release](https://github.com/yt-dlp/yt-dlp/releases/tag/2026.07.04),
  [release API record](https://api.github.com/repos/yt-dlp/yt-dlp/releases/tags/2026.07.04), and
  [full release commit](https://github.com/yt-dlp/yt-dlp/commit/fdec00e0bf530dc6c3cc7b1dd780e95d9ae460e9).

- The release source tarball is
  `31c32457d1a573a341bb0929386c624fe47339a5338829e6e9c9454bdfa7397a`, but upstream's
  license statement distinguishes that Unlicense archive from the combined GPLv3-or-later
  PyInstaller builds. The archive therefore cannot, by itself, document the Python, PyInstaller,
  and dependency source corresponding to the standalone executable.
- The tag's `uv.lock` records exact dependency artifacts and hashes, including PyInstaller 6.21.0
  and architecture-specific wheels; `pyproject.toml` describes the build dependency groups. See
  [`uv.lock`](https://github.com/yt-dlp/yt-dlp/blob/2026.07.04/uv.lock),
  [`pyproject.toml`](https://github.com/yt-dlp/yt-dlp/blob/2026.07.04/pyproject.toml), and the
  [release workflow](https://github.com/yt-dlp/yt-dlp/blob/2026.07.04/.github/workflows/release.yml).
- Two days after the release, upstream's
  [license-information correction](https://github.com/yt-dlp/yt-dlp/commit/b3854cc41bc906c905e3b0f7bb39755210acd6d1)
  substantially changed the inventory, corrected Linux/macOS applicability for several native
  libraries, added transitive components, changed the source-offer contact, and changed the
  PyInstaller command to embed `THIRD_PARTY_LICENSES.txt`. The tag's
  [PyInstaller script](https://github.com/yt-dlp/yt-dlp/blob/2026.07.04/bundle/pyinstaller.py)
  did not include that `--add-data` argument.

### Release interpretation and required evidence

For each architecture, retain and publish:

1. The exact executable, upstream `SHA2-256SUMS`, its signature, and any upstream attestations.
2. yt-dlp source at release commit `fdec00e0bf530dc6c3cc7b1dd780e95d9ae460e9`, including the
   Unlicense, `pyproject.toml`, `uv.lock`, PyInstaller script/hooks, and release workflow.
3. The exact Python source and build configuration, PyInstaller source and bootloader source, and
   source for every bundled Python and native dependency selected by the lock and platform. Record
   version/revision, source-archive SHA-256, license, and inclusion evidence per architecture.
4. Both the tag's `THIRD_PARTY_LICENSES.txt` and a reviewed corrected notice inventory. Do not
   silently replace the historical tag file: record the post-release correction commit and why
   the corrected inventory is included.
5. Captured `yt-dlp --version` and `yt-dlp --verbose --version` output from each shipped executable,
   plus an extracted PyInstaller contents inventory, so the source manifest can be checked against
   what the executable actually contains.

The upstream source-offer sentence is evidence about upstream's stated practice. It is not, by
itself, evidence that Loomarr has provided source next to its own redistributed object code. The
conservative beta closure is for Loomarr to host the exact source bundle itself.

## Notice and source-location inventory

The image should contain a stable directory such as `/usr/share/doc/loomarr/licenses/` with:

| Component | Files or notice required for the release record |
| --- | --- |
| Loomarr | Loomarr license and copyright notice |
| BtbN build machinery | BtbN MIT license and exact build commit |
| FFmpeg binary | GPLv3 text, FFmpeg copyright/attribution, exact build/configuration record, source URL |
| FFmpeg static dependency stack | Per-architecture generated inventory and every required copyright, attribution, and license text |
| yt-dlp source | Unlicense and exact release commit |
| yt-dlp standalone stack | GPLv3 text, tag notice file, reviewed corrected third-party notice inventory, exact source URL |
| Base-image packages | Debian-provided copyright/license material for installed runtime packages; retain package/version manifest |

An image-local `SOURCE_OFFER.md` should identify the Loomarr beta version and image manifest digest,
list the amd64 and arm64 source-bundle names and SHA-256 digests, and give direct HTTPS download
URLs. The same directions should be in the GitHub release notes and adjacent to GHCR installation
instructions. A generic project home page or a link to an upstream moving branch is insufficient
release evidence.

## Release manifest and retention mechanics

Publish a signed `release-source-manifest.json` for every beta with at least:

- Loomarr version, Git commit, multi-architecture OCI digest, and per-platform image digests;
- every redistributed binary's path, architecture, upstream URL, immutable release and commit,
  file size, and SHA-256;
- every corresponding-source bundle's URL, SHA-256, covered binary SHA-256 values, license set,
  and creation recipe;
- notice-file paths and SHA-256 values; and
- the retention policy and contact for a source request.

Keep the exact binary inputs and source assets under Loomarr's control for at least as long as any
covered image remains downloadable. BtbN's stated two-year monthly retention is shorter than a
safe project-controlled retention policy and does not cover its floating `latest` release. Counsel
should decide whether Loomarr additionally promises a three-year written offer; the network source
must remain available regardless while the image is distributed under the proposed section 6(d)
mechanism.

## Verification gate for a beta candidate

Run these checks in a clean release job for both `linux/amd64` and `linux/arm64`:

```sh
sha256sum -c release-binaries.sha256
sha256sum -c release-sources.sha256
ffmpeg -version
ffmpeg -buildconf
ffprobe -version
ldd /usr/local/bin/ffmpeg
ldd /usr/local/bin/ffprobe
yt-dlp --version
yt-dlp --verbose --version
```

The gate should additionally:

- reject a BtbN `latest` URL, an unrecorded upstream digest, `--enable-nonfree`, or a missing
  `--enable-gpl`/`--enable-version3` in captured configuration;
- compare the FFmpeg version identifier to the full source commit recorded in the manifest;
- ensure every enabled BtbN dependency stage has a retained source archive and license record for
  the relevant architecture;
- compare an extracted PyInstaller component inventory with the yt-dlp source and notice manifests;
- inspect the built image for all declared notice files and the exact `SOURCE_OFFER.md`;
- fetch every source URL without registry credentials and validate its digest; and
- run Loomarr's unchanged `make test-ffmpeg` gate against the candidate on both architectures.

A rebuild from the retained source should also be exercised before the first beta. Byte-for-byte
identity may require additional reproducibility work, but a successful rebuild is strong evidence
that the retained material is actually sufficient.

## Questions for qualified counsel

1. Does Loomarr's execution of separate GPL command-line programs constitute mere aggregation for
   the intended packaging and process interfaces, leaving Loomarr's own code distributable under
   MIT? No conclusion is made here.
2. Does the proposed same-release GitHub source asset plus in-image and GHCR-adjacent directions
   satisfy GPLv3 section 6(d) for every actual image distribution path, including mirrors and
   cached pulls?
3. What retention period applies after an image is delisted, and should Loomarr make a separate
   three-year written offer in addition to network source availability?
4. May Loomarr ever rely on an upstream source offer, or must it possess and serve the complete
   corresponding source throughout its distribution period?
5. Which dynamically loaded runtime libraries qualify as System Libraries, and which portions of
   the BtbN static stack require source and notices beyond the conservative inventory above?
6. Is the corrected post-release yt-dlp notice sufficient for the 2026.07.04 binaries, or must the
   actual binary contents be independently reconciled before redistribution? Does upstream's
   source-offer wording create any downstream reliance Loomarr may use?
7. Are additional codec patent, export-control, trademark, or attribution actions required in the
   intended beta geographies? Those issues are outside this copyright-license source review.

Until those questions are reviewed, release material should describe the license facts and source
availability without claiming that compliance has been legally certified.
