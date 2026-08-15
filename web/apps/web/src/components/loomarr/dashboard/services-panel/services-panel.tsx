import type { ServiceRow } from "@loomarr/api/models/serviceRow";
import { parseDocHref } from "@loomarr/core/anchor";
import { Link } from "@tanstack/react-router";
import { ArrowUpRight, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PanelRow } from "@/components/ui/panel-row";
import { cn } from "@/lib/utils";
import type { ServicesPanelProps } from "./services-panel.type";

const LABELS: Record<string, string> = {
  loomarr: "Loomarr core",
  media_server: "Media server",
  requester: "Requester",
  tunarr: "Tunarr",
  llm: "AI provider",
  tmdb: "TMDB",
  filler: "Filler",
  livetv: "Live TV wiring",
  tunarr_library: "Tunarr library",
};

const serviceLabel = (name: string): string => LABELS[name] ?? name;

const ServiceItem = ({ row, onFix }: { row: ServiceRow; onFix: (group: string) => void }) => {
  // Defensive neutral state for older servers. Current servers omit optional integrations that
  // have never been configured, but an empty failed row must still not become a red incident.
  const unconfigured = !row.ok && !row.target && !row.hint;
  const failed = !row.ok && !unconfigured;

  return (
    <PanelRow>
      <PanelRow.Main className="flex flex-wrap items-center gap-3">
        <span
          role="img"
          aria-label={row.ok ? "OK" : unconfigured ? "Not configured" : "Not responding"}
          className={cn(
            "size-1.5 shrink-0 rounded-full",
            row.ok ? "bg-lock" : unconfigured ? "bg-static-500" : "bg-onair",
          )}
        />
        <span className="w-32 shrink-0 text-sm">{serviceLabel(row.name)}</span>
        <span className="min-w-0 flex-1 truncate font-mono text-muted-foreground text-xs">
          {row.target || "not configured"}
        </span>
        {failed && row.hint ? (
          <p className="basis-full pl-7.5 text-muted-foreground text-xs">{row.hint}</p>
        ) : null}
      </PanelRow.Main>
      <PanelRow.Meta>
        <span
          className={cn(
            "shrink-0 font-mono text-xs",
            row.ok ? "text-lock" : unconfigured ? "text-static-400" : "text-onair",
          )}
        >
          {row.ok ? "pass" : unconfigured ? "not set" : "fail"}
        </span>
        {failed && row.settingsGroup ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => onFix(row.settingsGroup as string)}
            aria-label={`Fix ${serviceLabel(row.name)}`}
          >
            Fix →
          </Button>
        ) : null}
        {failed && row.docHref ? (
          <Link
            to="/help"
            search={parseDocHref(row.docHref)}
            className="inline-flex items-center gap-1 text-tune text-xs hover:underline"
          >
            Help <ArrowUpRight className="size-3" aria-hidden />
          </Link>
        ) : null}
      </PanelRow.Meta>
    </PanelRow>
  );
};

const ServicesPanel = ({ view, onFix, refreshing, className }: ServicesPanelProps) => (
  <section className={cn("overflow-hidden rounded-lg border border-border", className)}>
    <div className="flex flex-wrap items-baseline gap-2.5 border-border border-b px-4 py-3.5">
      <h2 className="font-medium text-sm">Services</h2>
      <span className="ml-auto flex items-center gap-1.5 font-mono text-muted-foreground text-xs">
        {refreshing ? <Loader2 className="size-3 animate-spin" aria-hidden /> : null}
        checked every 30s · the same checks Settings runs
      </span>
    </div>

    <ul>
      {[view.loomarr, ...(view.rows ?? [])]
        .filter((row): row is ServiceRow => Boolean(row))
        .map((row) => (
          <ServiceItem key={row.name} row={row} onFix={onFix} />
        ))}
    </ul>
  </section>
);

export { ServicesPanel };
