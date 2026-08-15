import type { SettingEntry } from "@loomarr/api/models/settingEntry";

// A minimal VALID SettingEntry. Same rule as `./channels` and `./users`: every required field the
// wire declares, nothing more.
//
// ⚠ `SettingEntry` requires EIGHT fields — `advanced`, `doc`, `group`, `key`, `kind`,
// `provenance`, `secret`, `set`. Tests wrote four of them:
//
//   { key: "tunarr.url", set: true, provenance: "db", secret: false }
//
// …and an untyped stub served it. `kind` in particular is what the Settings UI switches its
// control on, so an entry without one is not a thing the server can produce.
//
// The default is a plain non-secret string knob resolved from the database, which is what most
// callers mean by "this key is configured":
//
//   setting({ key: "tunarr.url", value: "http://tunarr:8000" })
//   setting({ key: "llm.api_key", secret: true, set: false })
const setting = (over: Partial<SettingEntry> = {}): SettingEntry => ({
  key: "tunarr.url",
  kind: "string",
  group: "playout",
  doc: "Where Tunarr lives.",
  provenance: "db",
  advanced: false,
  secret: false,
  set: true,
  ...over,
});

export { setting };
