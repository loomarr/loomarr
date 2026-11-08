import { Action, StatusWord } from "@loomarr/design-system";
import { ListGroup, PanelRow } from "@loomarr/ui";
import { Loader2 } from "lucide-react";
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
  sonarr: "Sonarr",
  radarr: "Radarr",
  tunarr: "Tunarr",
  tunarr_library: "Tunarr library",
  livetv: "Live TV guide",
  llm: "AI provider",
  tmdb: "TMDB",
  filler: "Filler",
};

// An unlisted key still reads as words ("new_probe" → "New probe"), never as the raw key (#2734).
const serviceLabel = (name: string): string => {
  const label = LABELS[name];
  if (label) return label;
  const words = name.replaceAll("_", " ");
  return `${words.charAt(0).toUpperCase()}${words.slice(1)}`;
};

const ServicesPanel = ({ view, onFix, onDiagnose, refreshing, className }: ServicesPanelProps) => {
  const rows = view.rows ?? [];

  return (
    <section className={cn("border-border overflow-hidden rounded-lg border", className)}>
      <div className="border-border flex items-baseline gap-2.5 border-b px-4 py-3.5">
        <h2 className="text-sm font-medium">Services</h2>
        <span className="text-muted-foreground ml-auto flex items-center gap-1.5 font-mono text-xs">
          {refreshing ? <Loader2 className="size-3 animate-spin" aria-hidden /> : null}
          checked every 30s · the same checks Settings runs
        </span>
      </div>

      <ListGroup framed={false}>
        {[view.loomarr, ...rows].filter(Boolean).map((row) => (
          <PanelRow
            key={row.name}
            meta={
              <>
                <StatusWord kind={row.ok ? "ok" : !row.target && !row.hint ? "notSetUp" : "attention"} />
                {/* ⚠ Only a FAILING row offers Fix — and only when the server said where its
                  settings live. Offering it on a healthy row would train the eye past it. */}
                {!row.ok && row.settingsGroup ? (
                  <Action
                    accessibilityLabel={`Fix ${serviceLabel(row.name)}`}
                    onPress={() => onFix(row.settingsGroup as string)}
                    variant="outline"
                  >
                    Fix →
                  </Action>
                ) : null}
                {!row.ok && onDiagnose ? (
                  <Action
                    accessibilityLabel={`View ${serviceLabel(row.name)} diagnostics`}
                    onPress={() => onDiagnose(row.name)}
                    variant="ghost"
                  >
                    Logs →
                  </Action>
                ) : null}
              </>
            }
          >
            <div className="flex items-center gap-3">
              <span className="w-32 shrink-0 text-sm">{serviceLabel(row.name)}</span>
              <span className="text-muted-foreground min-w-0 flex-1 truncate font-mono text-xs">
                {/* An unconfigured integration has no target. Saying so beats an empty cell,
                    which reads as a rendering bug rather than an answer. ⚠ A PASSING row with
                    no target is not unconfigured: TMDB has no target by design (#2734). */}
                {row.target || (row.ok ? "" : "not configured")}
              </span>
              {/* The server's actionable detail ("connection refused"), under the row where a
                  failed check sits — the panel's job is to explain, not just to flag. */}
              {!row.ok && row.hint ? (
                <p className="text-muted-foreground mt-1 basis-full text-xs">{row.hint}</p>
              ) : null}
            </div>
          </PanelRow>
        ))}
      </ListGroup>
    </section>
  );
};

export { ServicesPanel };
