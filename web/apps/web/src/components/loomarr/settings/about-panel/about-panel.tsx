import { cn } from "@/lib";
import type { AboutPanelProps } from "./about-panel.type";

// AboutPanel — Settings → System → About (§16, V12; v2 mock System → About).
//
// What the operator quotes in a bug report. Every row comes from the server: a version
// the frontend derived from its own bundle would describe the frontend, which is exactly
// the wrong answer when the two are out of step.
//
// ⚠ **Rows the server cannot fill are absent, not blank.** The mock draws Go runtime,
// uptime and schema version; `GET /v1/system/version` returns none of them. A row reading
// "Uptime —" is a promise the page cannot keep, and this codebase has been bitten before
// by UI asserting a fact nothing produces. When the endpoint grows those fields the rows
// appear here; until then the page shows what is true.

const AboutPanel = ({ version, backend, className }: AboutPanelProps) => {
  const rows: { key: string; value: string }[] = [
    { key: "Version", value: version.dirty ? `${version.version} (modified)` : version.version },
  ];
  if (version.commit) rows.push({ key: "Commit", value: version.commit });
  if (version.builtAt) rows.push({ key: "Built", value: version.builtAt });
  if (backend) rows.push({ key: "Database", value: backend });
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
