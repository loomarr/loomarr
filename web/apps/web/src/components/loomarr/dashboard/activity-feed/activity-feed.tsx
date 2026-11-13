import { formatRelative } from "@loomarr/core/format";
import { Surface, Text } from "@loomarr/design-system";
import { ListGroup, PanelRow } from "@loomarr/ui";
import { cn } from "@/lib/utils";
import type { ActivityFeedProps } from "./activity-feed.type";

// ActivityFeed — "what has Loomarr been doing?" (§12, V32; v2 mock Dashboard § Recent activity).
//
// ⚠ **Rendered from the PERSISTED table, never assembled from SSE frames.** The `activity`
// frame carries no payload precisely so this cannot be tempted: the bus drops frames for a
// slow subscriber by design, so a list built from them would be silently missing rows — the
// exact failure the table exists to prevent. The frame says "something happened"; this
// refetches (§8).
//
// Each line's text was composed at the WRITE point and stored, not templated here. A feed row
// is a historical record: re-rendering "CH 42 backfilled" against current data would let last
// week's entry change its wording when the channel is renamed.

// The dot colour per level. A closed map rather than a computed class, so a level the server
// should never send cannot silently render as no dot at all.
const LEVEL_DOT: Record<string, string> = {
  info: "bg-lock",
  warn: "bg-signal",
  error: "bg-onair",
};

const ActivityFeed = ({ entries, now = Date.now(), className }: ActivityFeedProps) => (
  <section className={cn("border-border overflow-hidden rounded-lg border", className)}>
    <Text as="h2" className="border-border border-b px-4 py-3.5" textRole="cardLabel">
      Recent activity
    </Text>

    {entries.length === 0 ? (
      // A fresh install has done nothing yet. Saying so beats an empty box, which reads as a
      // panel that failed to load.
      <Text as="p" className="px-4 py-6 text-center" textRole="compact" tone="secondary">
        Nothing yet. Activity shows up here as Loomarr acquires titles and builds channels.
      </Text>
    ) : (
      <ListGroup framed={false}>
        {entries.map((e) => (
          <PanelRow key={e.id}>
            <Surface
              alignItems="center"
              backgroundColor="transparent"
              borderWidth={0}
              flexDirection="row"
              gap={12}
            >
              <Text as="span" className="w-14 shrink-0 whitespace-nowrap" textRole="mono">
                {/* `at` is unix SECONDS, per the schema's epoch convention; formatRelative takes ms. */}
                {formatRelative(e.at * 1000, now)}
              </Text>
              <span
                role="img"
                aria-label={e.level}
                className={cn("size-1.5 shrink-0 rounded-full", LEVEL_DOT[e.level] ?? "bg-static-500")}
              />
              <Text as="span" className="min-w-0 flex-1" textRole="compact">
                {e.text}
              </Text>
            </Surface>
          </PanelRow>
        ))}
      </ListGroup>
    )}
  </section>
);

export { ActivityFeed };
