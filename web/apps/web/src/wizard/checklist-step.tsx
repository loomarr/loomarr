import { setupApi } from "@loomarr/api";
import { humanizeSettingKey } from "@loomarr/core";
import { RefreshCw } from "lucide-react";
import { ChecklistItem, ErrorState } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { REQUIRED_CHECKS } from "./steps";

// Wizard step 2 — the live connection checklist (§13). Backed by GET /v1/setup/status,
// which runs every probe server-side and returns {name, ok, hint, docHref} per check.
// A red check never blames: it shows the plain-language hint the BE supplied plus a deep
// link into the embedded Troubleshooting docs. Optional integrations (Seerr, AI, TMDB,
// filler) are reported but never block — the shortest honest path to a live channel is
// media server + Tunarr (config-design §6).
const ChecklistStep = () => {
  const status = setupApi.useSetupStatus();
  const checks = status.data?.status === 200 ? (status.data.data.checks ?? []) : [];

  if (status.error) return <ErrorState error={status.error} onRetry={() => status.refetch()} />;

  return (
    <div className="flex flex-col gap-4">
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
        {checks.length === 0 && !status.isFetching && (
          <li className="text-muted-foreground text-sm">No checks reported yet.</li>
        )}
      </ul>

      <p className="text-muted-foreground text-xs">
        {REQUIRED_CHECKS.map(humanizeSettingKey).join(" and ")} must pass to continue. The rest add features
        you can wire up later — Settings re-runs this checklist for the life of the install.
      </p>

      <Button
        variant="outline"
        className="w-fit"
        onClick={() => status.refetch()}
        disabled={status.isFetching}
      >
        <RefreshCw className={status.isFetching ? "animate-spin" : undefined} aria-hidden />
        {status.isFetching ? "Checking…" : "Re-run checks"}
      </Button>
    </div>
  );
};

export { ChecklistStep };
