import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PanelRow } from "@/components/ui/panel-row";
import { cn } from "@/lib/utils";
import type { ServicesPanelProps } from "./services-panel.type";

// ServicesPanel — "is anything broken?" (§12, V31; v2 mock Dashboard § Services).
//
// One row per configured integration: what it is, what it was pointed at, whether it
// answered, and — when it did not — where to go and fix it.
//
// ⚠ **This POLLS rather than taking an SSE frame, unlike the activity feed beside it.** A
// probe result is not a state change anyone observes: the server only learns it by making
// outbound calls (~730ms for the set). Pushing it would need a server-side timer doing that
// polling continuously, whether or not a browser is open — client polling stops when nobody
// is looking. It would also invert §8's rule, where a frame is droppable precisely because a
// GET can re-derive the truth; here the probe IS the truth.

// serviceLabel turns a probe key into what an operator calls it. Derived here rather than
// sent by the server: the key is the API's identifier and the label is presentation, and
// pushing copy through the wire would make a wording fix a backend deploy.
const LABELS: Record<string, string> = {
  loomarr: "Loomarr core",
  media_server: "Media server",
  requester: "Requester",
  tunarr: "Tunarr",
  llm: "AI provider",
  tmdb: "TMDB",
  filler: "Filler",
};

const serviceLabel = (name: string): string => LABELS[name] ?? name;

const ServicesPanel = ({ view, onFix, onDiagnose, refreshing, className }: ServicesPanelProps) => {
  const rows = view.rows ?? [];

  return (
    <section className={cn("overflow-hidden rounded-lg border border-border", className)}>
      <div className="flex items-baseline gap-2.5 border-border border-b px-4 py-3.5">
        <h2 className="font-medium text-sm">Services</h2>
        <span className="ml-auto flex items-center gap-1.5 font-mono text-muted-foreground text-xs">
          {refreshing ? <Loader2 className="size-3 animate-spin" aria-hidden /> : null}
          checked every 30s · the same checks Settings runs
        </span>
      </div>

      <ul>
        {[view.loomarr, ...rows].filter(Boolean).map((row) => (
          <PanelRow key={row.name}>
            <PanelRow.Main className="flex items-center gap-3">
              <span
                role="img"
                aria-label={row.ok ? "OK" : "Not responding"}
                className={cn("size-1.5 shrink-0 rounded-full", row.ok ? "bg-lock" : "bg-onair")}
              />
              <span className="w-32 shrink-0 text-sm">{serviceLabel(row.name)}</span>
              <span className="min-w-0 flex-1 truncate font-mono text-muted-foreground text-xs">
                {/* An unconfigured integration has no target. Saying so beats an empty cell,
                    which reads as a rendering bug rather than an answer. */}
                {row.target || "not configured"}
              </span>
              {/* The server's actionable detail ("connection refused"), under the row where a
                  failed check sits — the panel's job is to explain, not just to flag. */}
              {!row.ok && row.hint ? (
                <p className="mt-1 basis-full pl-7.5 text-muted-foreground text-xs">{row.hint}</p>
              ) : null}
            </PanelRow.Main>
            <PanelRow.Meta>
              <span className={cn("shrink-0 font-mono text-xs", row.ok ? "text-lock" : "text-onair")}>
                {row.ok ? "pass" : "fail"}
              </span>
              {/* ⚠ Only a FAILING row offers Fix — and only when the server said where its
                settings live. Offering it on a healthy row would train the eye past it. */}
              {!row.ok && row.settingsGroup ? (
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
              {!row.ok && onDiagnose ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => onDiagnose(row.name)}
                  aria-label={`View ${serviceLabel(row.name)} diagnostics`}
                >
                  Logs →
                </Button>
              ) : null}
            </PanelRow.Meta>
          </PanelRow>
        ))}
      </ul>
    </section>
  );
};

export { ServicesPanel };
