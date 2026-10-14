# expo-modules-jsi 58.0.0-canary-20260812-27f94d4: nested build without -quiet

Tracking: [#1816](https://github.com/loomarr/loomarr/issues/1816) (iPhone TestFlight beta).

ExpoModulesJSI compiles itself from source in a CocoaPods script phase, `[CP-User] Build
ExpoModulesJSI xcframework`, which runs a nested `xcodebuild -quiet build` of its Swift package.
Under Xcode 27.0 (27A266a), quiet mode reports every SwiftCompile task that emitted only warnings
as `error: the following command failed with exit code 0 but produced no further output`. The
nested build exits 0 and the framework is complete. `xcodebuild build` (the Simulator CI job)
prints those lines and still succeeds, but `xcodebuild archive` fails the parent script phase with
`Command PhaseScriptExecution emitted errors but did not return a nonzero exit code to indicate
failure` and ends in `ARCHIVE FAILED` (the first TestFlight dry run, workflow run 37164267848).
The same nested command without `-quiet` prints no error.

The patch drops `-quiet` from that one nested `xcodebuild` call. Compiler flags, the build hash
cache and the produced framework are unchanged; the build log carries the nested build's full
output. Remove it when upstream stops passing `-quiet` or Xcode stops reporting warning-only tasks
as errors.
