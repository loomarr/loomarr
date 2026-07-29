import { formatUptime } from "@loomarr/core";
import { cn } from "@/lib";
import type { AboutPanelProps } from "./about-panel.type";

// AboutPanel — Settings → System → About (§16, V12; v2 mock System → About).
//
// What the operator quotes in a bug report. Every row comes from the server: a version
// the frontend derived from its own bundle would describe the frontend, which is exactly
// the wrong answer when the two are out of step.
//
// ⚠ **Rows the server cannot fill are absent, not blank.** A row reading "Uptime —" is a
// promise the page cannot keep, and this codebase has been bitten before by UI asserting a
// fact nothing produces. Every field here is optional on the wire and every row is
// conditional: a source build has no commit, and a store-less boot has no schema version.
//
// **Uptime is derived here** from the server's `startedAt` instant. The server sends the
// instant rather than a duration on purpose (§7): an uptime computed server-side is stale
// the moment it is serialized.

const AboutPanel = ({ version, now = Date.now(), className }: AboutPanelProps) => {
  const rows: { key: string; value: string }[] = [
    { key: "Version", value: version.dirty ? `${version.version} (modified)` : version.version },
  ];
  if (version.commit) rows.push({ key: "Commit", value: version.commit });
  if (version.builtAt) rows.push({ key: "Built", value: version.builtAt });
  if (version.goVersion) {
    // One row, because "go1.23.4 · linux/amd64" is one fact when someone is diagnosing a
    // platform-specific problem, and the mock draws it that way.
    rows.push({
      key: "Go runtime",
      value: version.platform ? `${version.goVersion} · ${version.platform}` : version.goVersion,
    });
  }
  if (version.startedAt) {
    // ⚠ Derived here, from the server's INSTANT. The server deliberately sends startedAt
    // rather than a duration: an uptime computed server-side is stale the moment it is
    // serialized, and this render is where "how long has it been up" is actually asked.
    rows.push({ key: "Uptime", value: formatUptime(now - new Date(version.startedAt).getTime()) });
  }
  if (version.backend) {
    // The schema version rides the Database row: an operator diagnosing a migration
    // problem needs the backend and the applied version together, and two rows saying
    // "sqlite" and "20" separately makes them do the join.
    rows.push({
      key: "Database",
      value: version.schemaVersion ? `${version.backend} · schema ${version.schemaVersion}` : version.backend,
    });
  }
  rows.push({ key: "Status", value: version.ready ? "Ready" : (version.detail ?? "Not ready") });

  return (
    <section className={cn("overflow-hidden rounded-lg border border-border", className)}>
      <h2 className="sr-only">About this install</h2>
      <dl className="divide-y divide-border">
        {rows.map((r) => (
          <div key={r.key} className="flex items-center gap-4 px-4 py-2.5">
            <dt className="w-36 shrink-0 text-muted-foreground text-sm">{r.key}</dt>
            <dd className="min-w-0 flex-1 break-words font-mono text-sm">{r.value}</dd>
          </div>
        ))}
      </dl>
    </section>
  );
};

export { AboutPanel };
