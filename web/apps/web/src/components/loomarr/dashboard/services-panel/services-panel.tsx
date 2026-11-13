import { Action, ActivityIndicator, Stack, StatusWord, Text } from "@loomarr/design-system";
import { ListGroup, PanelRow } from "@loomarr/ui";
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
      <Stack align="baseline" className="border-border border-b px-4 py-3.5" direction="row" gap="2.5">
        <Text as="h2" textRole="cardLabel">
          Services
        </Text>
        <Stack align="center" className="ml-auto" direction="row" flexShrink={1} gap="1.5" render="span">
          {refreshing ? <ActivityIndicator decorative size="compact" tone="secondary" /> : null}
          <Text as="span" textRole="mono">
            checked every 30s · the same checks Settings runs
          </Text>
        </Stack>
      </Stack>

      <ListGroup framed={false}>
        {[view.loomarr, ...rows].filter(Boolean).map((row) => (
          <PanelRow
            key={row.name}
            meta={
              <>
                {/* The server's own verdict (#2781, the one Connections uses): "not OK only because
                    nothing about this service is set yet". Never guessed from target or hint —
                    an unset optional service carries a hint too (#3403). */}
                <StatusWord kind={row.ok ? "ok" : row.notSetUp ? "notSetUp" : "attention"} />
                {/* ⚠ Only a FAILING row offers Fix — and only when the server said where its
                  settings live. Offering it on a healthy row would train the eye past it. A
                  not-set-up row keeps it: that is the page that sets the service up. */}
                {!row.ok && row.settingsGroup ? (
                  <Action
                    accessibilityLabel={`Fix ${serviceLabel(row.name)}`}
                    onPress={() => onFix(row.settingsGroup as string)}
                    variant="outline"
                  >
                    Fix →
                  </Action>
                ) : null}
                {!row.ok && !row.notSetUp && onDiagnose ? (
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
            <Stack align="center" direction="row" gap="control">
              <Text as="span" className="w-32 shrink-0" textRole="compact">
                {serviceLabel(row.name)}
              </Text>
              <Text as="span" className="min-w-0 flex-1 truncate" textRole="mono">
                {/* An unconfigured integration has no target. Saying so beats an empty cell,
                    which reads as a rendering bug rather than an answer. ⚠ A PASSING row with
                    no target is not unconfigured: TMDB has no target by design (#2734). */}
                {row.target || (row.ok ? "" : "not configured")}
              </Text>
              {/* The server's actionable detail ("connection refused"), under the row where a
                  failed check sits — the panel's job is to explain, not just to flag. */}
              {!row.ok && row.hint ? (
                <Text as="p" className="mt-1 basis-full" textRole="hint" tone="secondary">
                  {row.hint}
                </Text>
              ) : null}
            </Stack>
          </PanelRow>
        ))}
      </ListGroup>
    </section>
  );
};

export { ServicesPanel };
