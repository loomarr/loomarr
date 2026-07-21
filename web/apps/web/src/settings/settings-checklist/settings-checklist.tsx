import { setupApi } from "@loomarr/api";
import { humanizeSettingKey } from "@loomarr/core";
import { RefreshCw } from "lucide-react";
import { ChecklistItem, ErrorState } from "@/components/loomarr";
import { Button } from "@/components/ui";

// The same checklist the first-run wizard walks, re-runnable for the life of the install
// (§13: "the checklist is executable documentation; the troubleshooting page is its
// narrative twin"). It sits at the top of Connections because that is where an operator
// lands when something has stopped working, and a red check deep-links to the fix.
const SettingsChecklist = () => {
  const status = setupApi.useSetupStatus({ query: { retry: false } });
  const checks = status.data?.status === 200 ? (status.data.data.checks ?? []) : [];

  if (status.error) {
    return <ErrorState error={status.error} onRetry={() => status.refetch()} />;
  }

  return (
    <section className="flex flex-col gap-3 rounded-lg border border-border bg-card p-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="font-semibold text-lg">Connection checklist</h2>
          <p className="text-muted-foreground text-sm">
            Live-tested against each service. A red check tells you what to fix and links to the docs for it.
          </p>
        </div>
        <Button variant="outline" onClick={() => status.refetch()} disabled={status.isFetching}>
          <RefreshCw className={status.isFetching ? "animate-spin" : undefined} aria-hidden />
          {status.isFetching ? "Checking…" : "Re-run checks"}
        </Button>
      </div>

      <ul className="flex flex-col gap-2">
        {checks.map((check) => (
          <li key={check.name}>
            <ChecklistItem
              name={humanizeSettingKey(check.name)}
              status={status.isFetching ? "running" : check.ok ? "pass" : "fail"}
              hint={check.hint}
              docHref={check.docHref}
            />
          </li>
        ))}
      </ul>
    </section>
  );
};

export { SettingsChecklist };
