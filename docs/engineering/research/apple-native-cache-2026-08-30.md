# Apple native-build cache research

**Compiled 2026-08-30.** This brief evaluates a safe, bounded cache for Loomarr's iOS and tvOS
simulator gates. It uses GitHub, Apple, Expo, and repository-owned primary sources. Cache inventory
is a dated API snapshot; product behavior can change. No CI or application code was changed for this
research.

## Decision

Prototype **Xcode 26 compilation caching**, not a transported full DerivedData tree, and keep Expo's
existing narrow `ExpoModulesJSI` binary cache. The prototype should cache only Xcode's compilation
content-addressable store, publish it from a trusted default-branch warmer, and restore it read-only
in pull-request and merge-queue jobs. Every cache miss, rejected entry, or restore failure must fall
through to the current Release build, install, launch, architecture, and liveness proof.

Do not implement the workflow change until the remaining supported-toolchain and observability
cases in [Validation plan](#validation-plan) are automated. The local prototype proved the core
archive/restore, mobile, TV, source-change, corruption, and cold-fallback mechanics, but the current
macOS beta host cannot run the installed Xcode 26 simulator runtimes. The app experiment therefore
used Xcode 27 beta and must be repeated on GitHub's supported `macos-26` image before a cache is
published. Apple documents compilation caching as useful for clean builds and branch switches, but
does not document a portable contract for copying the rest of DerivedData between machines. That
makes compilation caching the supported seam and full DerivedData an unproven alternative.

This recommendation is a Loomarr inference from the sourced facts below, not a claim that Apple or
GitHub recommends this exact workflow topology.

## Current repository facts

Loomarr currently has two independent reusable workflows:

- [mobile](../../../.github/workflows/ci-apple-mobile.yml) and
  [TV](../../../.github/workflows/ci-apple-tv.yml) run on `macos-26`, fingerprint `xcodebuild
  -version`, and cache the pnpm/CocoaPods download stores plus `ExpoModulesJSI`'s final
  `Products` directory;
- both call [the same verifier](../../../web/scripts/test-apple-client.sh), which regenerates the
  native directory with `expo prebuild --clean`, performs a Release simulator build, asserts that
  the executable has exactly the host architecture, installs and launches it, waits five seconds,
  and proves the process is still alive;
- [the top-level workflow](../../../.github/workflows/ci.yml) runs for `pull_request`,
  `merge_group`, and manual dispatch, but not for a push to `main`;
- `ONLY_ACTIVE_ARCH=YES` is scoped through
  [the simulator xcconfig](../../../web/scripts/apple-simulator.xcconfig).

The GitHub cache REST API reported **65 active entries using 10,618,722,783 bytes** at the time of
this review. Twenty Apple entries used **1,234,474,861 bytes**. All twenty belonged to five distinct
`refs/heads/gh-readonly-queue/main/...` refs; there was no Apple entry under `refs/heads/main`.
Those figures came from `GET /repos/loomarr/loomarr/actions/cache/usage` and
`GET /repos/loomarr/loomarr/actions/caches`, whose official endpoints support repository usage,
listing, and deletion ([GitHub cache REST API](https://docs.github.com/en/rest/actions/cache)).

**Inference:** the current Apple cache writes cannot help a later merge group. They are identical
dependency keys saved repeatedly into sibling temporary refs, consume more than 1.2 GB, and compete
inside a repository already above the default 10 GB allowance. A one-time manual dispatch on `main`
could seed the present dependency keys, but an evolving compiler cache needs a deliberate trusted
writer and retention policy.

## GitHub cache semantics

### Ref scope makes merge-group writes ephemeral

GitHub scopes an Actions cache to its key, cache version, and branch. A workflow can restore entries
from its current branch and the default branch, but cannot restore from sibling branches. Entries
created by pull-request workflows live under the synthetic merge ref and are available only to
reruns of that pull request ([dependency caching reference](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching#restrictions-for-accessing-a-cache)).
Merge queues likewise create temporary `gh-readonly-queue/{base_branch}` branches with distinct
SHAs ([merge queue documentation](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/configuring-pull-request-merges/managing-a-merge-queue#configuring-continuous-integration-ci-workflows-for-merge-queues)).

**Inference:** separate merge groups are sibling branch scopes. A cache created by one group cannot
be the cross-group source. A cache created on `main` is the only ordinary Actions-cache scope all of
them can read.

GitHub permits only a bounded set of trusted event types—including `push`, `workflow_dispatch`, and
`schedule`—to write into the default branch's scope. Other triggers resolving to the default branch
receive read-only access, and GitHub explicitly suggests a trusted default-branch workflow as the
writer and `actions/cache/restore` for consumers
([low-trust cache access](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching#cache-access-for-low-trust-workflow-triggers)).

### Entries are immutable and quota is shared

An existing key's contents cannot be changed; a refresh requires a new key. Prefix restore returns
the most recently created match
([cache key matching](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching#cache-key-matching)).
GitHub removes entries untouched for more than seven days and, at the repository limit, evicts from
least recently accessed to most recently accessed. The default limit is 10 GB, shared by every cache
in the repository
([usage and eviction policy](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching#usage-limits-and-eviction-policy)).

GitHub's conceptual guidance says jobs should always be able to regenerate cached files when the
cache is unavailable and warns that restored cache contents are untrusted input
([dependency caching concepts](https://docs.github.com/en/actions/concepts/workflows-and-actions/dependency-caching)).

**Inference:** a compiler cache needs a rolling unique save key, an exact compatibility prefix for
restore, restore-only consumers, and explicit deletion of old default-branch generations. Merely
adding `${{ github.run_id }}` without retention would reproduce Loomarr's previous cache-thrashing
failure. Cached paths must contain neither scripts nor credentials, and all restored outputs must
still pass the normal compiler and runtime proof.

## Expo semantics

### `prebuild --clean` and the Xcode build cache are different layers

Expo Continuous Native Generation says `prebuild --clean` deletes the native directories before
regenerating them; a non-clean prebuild layers changes and may not produce the same result. Expo
advises a clean prebuild when keeping generated native code synchronized
([CNG clean behavior](https://docs.expo.dev/workflow/continuous-native-generation/#clean)). Loomarr's
clean generation is therefore a correctness property and should remain.

Expo CLI documents `--no-build-cache` separately: on iOS it clears native derived data before
building. It also documents `--binary` as **skipping the build entirely** and only attempting to
install a supplied binary
([Expo CLI compiling options](https://docs.expo.dev/more/expo-cli/#compiling)). In SDK 57 source,
`buildCache === false` appends `clean build` to the `xcodebuild` invocation; the ordinary path does
not append `clean`
([SDK 57 `XcodeBuild.ts`](https://github.com/expo/expo/blob/sdk-57/packages/%40expo/cli/src/run/ios/XcodeBuild.ts#L1701-L1715)).

**Inference:** `--binary` is unsuitable as the authoritative gate because it bypasses compilation
of the merge-group source. `--no-build-cache` is useful for the cold-control lane, but a clean
native-project generation does not itself prohibit Xcode's compilation-result cache.

### ExpoModulesJSI already owns a narrower binary cache

SDK 57's `ExpoModulesJSI` CocoaPods phase deliberately runs every build and delegates no-op
detection to an internal hash
([podspec](https://github.com/expo/expo/blob/sdk-57/packages/expo-modules-jsi/apple/ExpoModulesJSI.podspec)).
Its build script hashes its Swift/C++ sources, API notes, scripts, actual JSI header contents,
`PODS_ROOT`, `RN_ROOT`, and `swiftc --version`; Expo states that compiler changes must invalidate
the slice because Swift modules and Clang module caches are toolchain-bound
([SDK 57 build script](https://github.com/expo/expo/blob/sdk-57/packages/expo-modules-jsi/apple/scripts/build-xcframework.sh#L1262-L1347)).

**Inference:** keep caching only `Products` for this package. Do not broaden that entry to its
private `.DerivedData`, `.build`, or `.swiftpm` directories, and do not attempt to replace Expo's
own input validation with a Loomarr-maintained partial list. The workflow's outer key must retain
the exact Swift/Xcode toolchain and dependency lock fingerprint.

## Xcode 26 compilation caching

Apple introduced opt-in compilation caching in Xcode 26 for Swift and C-family languages. Apple
describes it as caching compilation results for a set of source inputs and identifies clean builds
and branch switching as the workflows that benefit most. It is enabled by the **Enable Compilation
Caching** build setting
([Xcode 26 release notes](https://developer.apple.com/documentation/xcode-release-notes/xcode-26-release-notes#Build-System)).
Apple's build-setting reference names the settings
`COMPILATION_CACHE_ENABLE_CACHING` and `COMPILATION_CACHE_ENABLE_DIAGNOSTIC_REMARKS`
([build settings reference](https://developer.apple.com/documentation/xcode/build-settings-reference#Compilation-Caching)).

The installed supported toolchain, Xcode 26.6 (build 17F113), corroborates those public docs:

- `COMPILATION_CACHE_ENABLE_CACHING` is opt-in and described as caching compilation results for a
  particular input set;
- `swiftc -help` exposes `-cache-compile-job`, `-cas-path`, and `-Rcache-compile-job`;
- the per-user store is `~/Library/Developer/Xcode/DerivedData/CompilationCache.noindex`;
- `llvm-cas` can validate and prune an on-disk content-addressable store.

The installed Xcode specifications also recognize `COMPILATION_CACHE_CAS_PATH` as a way to redirect
the store. Unlike the two public settings above, this path override is an observed implementation
detail rather than a documented compatibility promise. These are observations from Apple's
installed command-line tools and Xcode specifications, not a promise of an undocumented file
format. The open-source Swift driver independently computes output cache keys from compiler command
lines and contributing inputs
([Swift driver `CompileJob.swift`](https://github.com/swiftlang/swift-driver/blob/main/Sources/SwiftDriver/Jobs/CompileJob.swift)).

**Inference:** the global compilation store is a much stronger cache boundary than a project-wide
DerivedData archive. Xcode/Swift, not a hand-maintained GitHub key, decides whether a source input
has a reusable result. However, copying that store through Actions is still an integration that
Apple does not expressly document; local restore, validation, corruption, and clean-fallback tests
remain mandatory.

## Why not full DerivedData first

Full DerivedData contains build products, intermediates, module caches, dependency records, and the
build database. Apple documents what `-derivedDataPath` selects, but does not publish a guarantee
that this whole tree can be transferred between clean hosted machines. Apple also treats deleting
DerivedData as a supported recovery for classes of Xcode build failure
([Xcode 26 release notes](https://developer.apple.com/documentation/xcode-release-notes/xcode-26-release-notes)).

The current verifier does not pin `-derivedDataPath`; Expo locates the product in Xcode's selected
DerivedData and copies the `.app` to `LOOMARR_APPLE_BUILD_DIR`. CNG also regenerates the Xcode
project and Pods workspace on every gate. Those facts make a full-tree archive sensitive to
workspace paths, generated project identity, Pods integration, Xcode build database format, SDK,
configuration, architecture, and toolchain.

**Inference:** do not ship full DerivedData on the evidence available. It would require a much
larger archive, a broader invalidation contract, and a cold retry for a portability contract Apple
does not state. Reconsider it only if compilation caching produces too few hits and a two-machine
experiment proves correctness and net wall-time benefit after upload/download.

## Local prototype findings

The prototype preserved `expo prebuild --clean` and used a different empty artifact/build root for
every authoritative run. The installed Xcode 26.6 toolchain supplied the settings/specification
evidence, but cannot launch its simulator runtimes on this macOS 27 beta host. Native build
mechanics were therefore exercised with Xcode 27 beta; a temporary version shim bypassed only
Loomarr's Xcode-26 guard and did not intercept build commands. This is useful integration evidence,
not a substitute for the required hosted Xcode 26 proof.

| Experiment | Result |
| --- | --- |
| Full DerivedData | A 1.7 GB mobile tree compressed to 470 MB, but an identical verifier rerun remained effectively cold because `expo prebuild --clean` regenerated the native project and Pods workspace. Skipping prebuild made a direct Expo rerun fast, confirming that the apparent reuse depended on weakening Loomarr's clean-generation contract. |
| Mobile compilation CAS | An empty-store run and two fresh-build-root restores all passed Release build, arm64-only assertion, install, launch, screenshot, and liveness. Restore runs were materially faster than the empty-store run even though the clean prebuild remained. |
| Archive portability | A populated 1.1 GB mobile store compressed to 516 MB in 1.81 seconds, restored in 1.91 seconds, passed `llvm-cas --validate --check-hash`, and then passed the complete mobile proof from a fresh build root. |
| Shared mobile + TV store | TV populated the restored mobile store and passed the complete proof. The combined 2.0 GB store compressed to 986 MB, restored and validated, then passed the complete TV proof from a fresh build root. The restored TV run was materially faster than population. |
| Native source change | A harmless comment changed one compiled `RNSVGLength.mm` input. A fresh-root mobile build passed every runtime assertion and added new validated CAS content. Expo's formatted console log showed the compile command but stripped Xcode's diagnostic cache remark, so the prototype did not prove the exact hit/miss count. |
| Corrupt archive and cold fallback | Truncating a copy of the combined archive made `zstd -t` reject it as a premature end. With compilation caching disabled and a fresh build root, the complete TV proof still passed. The earlier mobile no-cache control also passed completely. |
| Compatibility fingerprint | A shell prototype hashed OS, architecture, exact Xcode and Swift output, both simulator SDK descriptions, build/cache mode, `pnpm-lock.yaml`, and a schema constant. Perturbing each field independently selected a different SHA-256 prefix. |

The uncompressed combined CAS remained smaller than either full mobile or TV DerivedData tree alone
and—more importantly—survived clean native regeneration. That, rather than a particular timing
number, is the decisive result. `SWIFT_ENABLE_EXPLICIT_MODULES=YES` was necessary for Swift caching;
without it Xcode emitted a warning and only the C-family portion could participate.

The combined compressed candidate is still large relative to GitHub's shared cache allowance. Two
steady-state generations would consume roughly 2 GB, and saving a replacement temporarily requires
room for another generation. Loomarr was already above the default 10 GB repository allowance at
inventory time. The implementation must therefore make available headroom a precondition, set a
compressed-size ceiling from the prototype, and skip publishing rather than trigger unrelated LRU
evictions.

## Proposed cache contract

This is the implementation shape to validate, not yet workflow code.

### Writer and readers

1. A trusted `workflow_dispatch` cache-warmer on `main` runs mobile and TV **sequentially** with
   compilation caching enabled, then saves one shared compilation store. A later `push`-to-main
   trigger may replace manual dispatch only after its runner cost is understood.
2. Pull-request and `merge_group` jobs use `actions/cache/restore` only. They never save compiler
   state into temporary refs.
3. A miss, timeout, invalid archive, failed CAS validation, or unavailable cache service continues
   with an empty store and the ordinary native build.
4. The writer saves only after both Release install-launch-liveness gates pass.

The shared writer avoids two jobs racing to publish partial mobile/TV stores. It also lets identical
Swift/C-family inputs deduplicate in one CAS. Whether that materially reduces size is a measurement,
not an assumption.

### Compatibility fingerprint

The outer restore prefix should be an explicit schema version plus a SHA-256 over:

- `runner.os` and `runner.arch`;
- exact `xcodebuild -version` output;
- exact `xcrun swiftc --version` output;
- `xcodebuild -version -sdk iphonesimulator` and `-sdk appletvsimulator` output;
- the cache mode/build settings: Release, simulator, `ONLY_ACTIVE_ARCH=YES`, and
  `COMPILATION_CACHE_ENABLE_CACHING=YES`;
- the Expo SDK/native generator generation (`web/pnpm-lock.yaml`) and a Loomarr cache-schema
  constant.

Use a unique trusted-writer suffix such as `${{ github.run_id }}` for the save key and restore only
from the compatibility prefix. Source files do **not** belong in the outer prefix: Xcode's
content-addressed keys are the source-level invalidation mechanism, and hashing all sources outside
the CAS would turn almost every meaningful PR into a total miss. Any change to toolchain, SDK,
architecture, platform settings, or cache schema must select a different prefix before restore.

### Bounded retention

- Retain at most **two** successful default-branch compiler-cache generations for the active
  compatibility fingerprint: newest plus rollback. Begin with one retained generation unless the
  repository can demonstrate enough headroom for two plus the transient replacement upload.
- Delete older generations only after the new cache save succeeds.
- Refuse to save if the compressed candidate exceeds a measured ceiling chosen after the prototype;
  do not guess the ceiling from uncompressed disk use.
- Report restored bytes, save bytes, restore/save duration, CAS validation, cache remarks, compile
  duration, and the repository-wide cache total in the step summary.
- Remove the existing Apple saves from PR/merge-group refs once a default-branch reader is proven;
  keep the pnpm/CocoaPods entries restore-only there as well.

The exact byte ceiling is unresolved. The repository is already beyond the default quota, so it
must be set from measured compressed CAS size and leave headroom for Go, pnpm, Gradle, CodeQL, and
release caches.

### Clean fallback

The native verifier needs a cache mode with these semantics:

- `warm`: enable compilation caching and consume a restored store;
- on a cache-related validation or build failure, quarantine the restored store and retry **once**
  with compilation caching disabled plus Expo's `--no-build-cache`;
- `cold`: skip restore, disable compilation caching, pass `--no-build-cache`, and run the same
  mobile/TV Release install-launch-liveness assertions;
- never use `--binary` for authoritative verification;
- keep architecture, screenshot, relaunch, PID parsing, five-second liveness, and failure logs
  identical in every mode.

The retry must retain the first attempt's logs and identify whether the cold retry passed. Whether a
warm-failure/cold-pass result should fail CI or emit a warning is unresolved; the safer initial
policy is to fail the cache warmer (so it cannot publish) while allowing the merge-group product
proof to be decided explicitly in the implementation issue.

## Validation plan

Automate these experiments on the supported Xcode 26 toolchain before changing CI. Use separate
empty artifact/build roots for each case and record wall time plus `-showBuildTimingSummary` and raw
compilation-cache diagnostic remarks; timing is explanatory, not the pass criterion.

| Case | Required evidence |
| --- | --- |
| Mobile cold | **Locally proven mechanically.** Repeat on `macos-26`: empty project DerivedData and compiler store; Release build/install/launch/liveness and arm64-only assertion pass. |
| Mobile warm | **Locally proven except raw remarks.** Repeat on `macos-26`; capture unfiltered diagnostics demonstrating real cache hits. |
| TV cold | **Locally proven mechanically.** Repeat on `macos-26` for tvOS. |
| TV warm | **Locally proven except raw remarks.** Repeat on `macos-26`; capture unfiltered cache hits. |
| Source invalidation | **Partially proven.** The changed native input produced new validated CAS content and a live app; automated raw diagnostics must prove that input misses while unaffected inputs hit. |
| Dependency invalidation | **Fingerprint logic proven, build case pending.** Change the lock/native graph; the outer key must change and a cold build must pass. |
| Toolchain invalidation | **Fingerprint logic proven.** Automate the fixture and prove restore lookup misses before extraction. |
| SDK/platform invalidation | **Fingerprint logic proven.** Automate both SDK/platform fixtures and prove restore lookup misses before extraction. |
| Corrupt archive | **Locally proven mechanically.** Automate rejection, quarantine, one cold retry, and the rule preventing a damaged save. |
| Missing cache | **Mobile and TV cold controls proven.** Automate an unknown-key run through the same fallback path. |
| Portability | **Locally proven across fresh directories.** Repeat across distinct hosted machines/users on `macos-26`. |
| Bounds | **Candidate measured.** Automate compressed-size refusal and fixture-test retention at one/two generations plus transient-upload headroom. |

After local success, seed one default-branch cache with manual dispatch and run two distinct
merge-group refs. Both must report restoring the same `refs/heads/main` generation. GitHub's cache
REST listing is the authoritative proof that neither merge group saved a sibling-scoped compiler
cache and that retention stayed within bounds.

## Evidence-backed implementation plan

1. Add a small repository-owned fingerprint/validation script with unit fixtures for toolchain,
   SDK, architecture, configuration, cache-schema, and missing-command failure.
2. Add opt-in compilation-cache settings and diagnostic remarks to the Apple simulator xcconfig;
   preserve `ONLY_ACTIVE_ARCH=YES`.
3. Add `warm` and `cold` modes plus a single quarantined clean retry to the Apple verifier; extend
   its source-contract tests before running native builds.
4. Repeat the full validation matrix on a `macos-26` hosted runner and capture raw cache remarks,
   compressed-store size, and transfer timings. Treat the local Xcode 27 prototype only as
   pre-implementation mechanics evidence.
5. Add a trusted, manually dispatched default-branch warmer that builds mobile then TV and saves
   only after both pass. Give only this workflow `actions: write`.
6. Make PR/merge-group Apple jobs restore-only and remove their Apple cache saves. Preserve ordinary
   dependency installation and every runtime assertion.
7. Add post-save default-branch retention cleanup, limited by exact key prefix and ref, retaining at
   most two generations. Test the selector against fixtures before granting deletion permission.
8. Seed from `main`, prove reuse from two distinct merge groups, compare end-to-end critical path
   including transfers, and either keep the cache or revert it if it does not deliver a repeatable
   net benefit.

Each step should be independently reviewable. Workflow code should not land until steps 1–4 are
green in their stated local or hosted environment.

## Unresolved questions

- What compressed-size ceiling and retention count leave enough measured repository headroom for a
  replacement upload without evicting unrelated live caches? The local combined candidate was 986
  MB, but GitHub's archive and hosted toolchain may differ.
- Does `actions/cache` archive/restore preserve every CAS property needed by Xcode 26.6 across two
  distinct hosted runners? Local tar/zstd transfer and `llvm-cas` validation passed.
- How many real cache hits remain after Expo's clean project regeneration and CocoaPods integration?
  Expo's formatter hid the diagnostic remarks even though restored runs improved materially.
- Does the nested `ExpoModulesJSI` build inherit Loomarr's compilation-cache setting? It currently
  runs through `env -i` and its own `xcodebuild`; its existing final-XCFramework cache should make
  this irrelevant on a hit, but the prototype must observe it.
- Should a merge-group warm failure followed by a clean pass fail the required check, or pass while
  opening telemetry/maintenance work? The cache warmer must fail and must never publish in either
  policy.
- Should the trusted writer be manual-only, scheduled, or triggered after an Apple-affecting merge?
  The answer depends on measured cache aging and whether warming costs less runner time than it
  saves across later groups.
- Is one combined CAS materially smaller and more useful than separate iOS/tvOS stores after
  compression? Use measurements, not platform-name assumptions.
- Can the repository reduce existing main-scope cache usage enough to retain two compiler-cache
  generations without purchasing more cache storage?
