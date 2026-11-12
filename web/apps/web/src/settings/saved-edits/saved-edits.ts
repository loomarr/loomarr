import type { settingsPatchResponse } from "@loomarr/api/endpoints/settings";
import { SettingResultStatus } from "@loomarr/api/models/settingResultStatus";
import { unwrap } from "@loomarr/api/unwrap";

// What a settings save actually kept. A 200 does not mean every key was written: the server
// answers per key (saved, invalid, pinned), so "the save succeeded" is a question about each
// submitted key. Every save-then-test path (Settings, the wizard's Connections and Playout steps)
// asks it here, so a rejected value stays editable and Test never runs against what was NOT saved.

// The submitted keys the server durably saved; none for a failed request.
const savedKeysOf = (response: settingsPatchResponse, edits: Record<string, string>): string[] => {
  const results = unwrap(response, (body) => body.results) ?? [];
  return Object.keys(edits).filter((key) =>
    results.some((result) => result.key === key && result.status === SettingResultStatus.saved),
  );
};

// True when there was something to save and every submitted key was saved.
const everyEditSaved = (response: settingsPatchResponse, edits: Record<string, string>): boolean => {
  const submitted = Object.keys(edits);
  return submitted.length > 0 && savedKeysOf(response, edits).length === submitted.length;
};

export { everyEditSaved, savedKeysOf };
