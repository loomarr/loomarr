import type { SystemVersionOutputBody } from "@loomarr/api/models/systemVersionOutputBody";
import { formatUptime } from "@loomarr/core/format";
import type { AboutDetail } from "@loomarr/ui";

// What someone quotes in a bug report (§16, V12), from GET /v1/system/version alone: a version the
// frontend derived from its own bundle would describe the frontend, the wrong answer when the two
// are out of step.
//
// ⚠ Rows the server cannot fill are ABSENT, not blank: a source build has no commit and a store-less
// boot no schema version, and "Commit —" is a promise the page cannot keep.

/** The release as the page names it: a build from a modified tree never reports as the clean tag. */
const aboutVersion = (v: SystemVersionOutputBody) => (v.dirty ? `${v.version} (modified)` : v.version);

const aboutDetails = (v: SystemVersionOutputBody, now: number): AboutDetail[] => {
  const rows: AboutDetail[] = [{ label: "Version", value: aboutVersion(v) }];
  if (v.commit) rows.push({ label: "Commit", value: v.commit });
  if (v.builtAt) rows.push({ label: "Built", value: v.builtAt });
  if (v.goVersion) {
    // One row: "go1.23.4 · linux/amd64" is one fact when diagnosing a platform problem.
    rows.push({ label: "Go runtime", value: v.platform ? `${v.goVersion} · ${v.platform}` : v.goVersion });
  }
  if (v.startedAt) {
    // ⚠ Derived here from the server's INSTANT: an uptime computed server-side is stale the moment
    // it is serialized.
    rows.push({ label: "Uptime", value: formatUptime(now - new Date(v.startedAt).getTime()) });
  }
  if (v.backend) {
    // The applied schema rides the Database row: a migration problem needs both together.
    rows.push({
      label: "Database",
      value: v.schemaVersion ? `${v.backend} · schema ${v.schemaVersion}` : v.backend,
    });
  }
  rows.push({ label: "Status", value: v.ready ? "Ready" : (v.detail ?? "Not ready") });
  return rows;
};

export { aboutDetails, aboutVersion };
