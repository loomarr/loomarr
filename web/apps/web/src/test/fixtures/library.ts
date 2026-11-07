import type { LibraryStatusOutputBody } from "@loomarr/api/models/libraryStatusOutputBody";

// A minimal VALID library status — every required field the wire declares: one folder in use, the
// scanner idle with nothing waiting. Spread it and override only what a test asserts on:
//
//   libraryStatus({ health: "needs_folders", inUse: 0 })
//
// Keep it honest: a new required field on the wire should fail to typecheck here.
const libraryStatus = (over: Partial<LibraryStatusOutputBody> = {}): LibraryStatusOutputBody => ({
  health: "ok",
  inUse: 1,
  offline: 0,
  toConfirm: 0,
  files: 0,
  headersPending: 0,
  problems: 0,
  held: [],
  hooks: [],
  scan: {
    enabled: true,
    stage: "idle",
    headersDone: 0,
    headersTotal: 0,
    paused: false,
    arrIncomplete: false,
  },
  ...over,
});

export { libraryStatus };
