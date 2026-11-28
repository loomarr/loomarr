import { formatRelative } from "@loomarr/core/format";
import { Stack, Surface, Text } from "@loomarr/design-system";
import { ListGroup, PanelRow } from "@loomarr/design-system/rows";
import type { ActivityFeedProps } from "./activity-feed.type";

// ActivityFeed — "what has Loomarr been doing?" (§12, V32; v2 mock Dashboard § Recent activity).
//
// ⚠ **Rendered from the PERSISTED table, never assembled from SSE frames.** The `activity`
// frame carries no payload precisely so this cannot be tempted: a client misses every frame
// sent while it reconnects, so a list built from them would be silently missing rows — the
// exact failure the table exists to prevent. The frame says "something happened"; this
// refetches (§8).
//
// Each line's text was composed at the WRITE point and stored, not templated here. A feed row
// is a historical record: re-rendering "CH 42 backfilled" against current data would let last
// week's entry change its wording when the channel is renamed.

// The dot colour per level. A closed map rather than a computed class, so a level the server
// should never send cannot silently render as no dot at all.
const LEVEL_DOT = {
  info: "$stateSuccess",
  warn: "$actionPrimary",
  error: "$guideOnAir",
} as const;

const ActivityFeed = ({ entries, now = Date.now() }: ActivityFeedProps) => (
  <Stack borderColor="$borderDecorative" borderRadius={8} borderWidth={1} overflow="hidden" render="section">
    <Stack borderBottomWidth={1} borderColor="$borderDecorative" paddingHorizontal={16} paddingVertical={14}>
      <Text as="h2" textRole="cardLabel">
        Recent activity
      </Text>
    </Stack>

    {entries.length === 0 ? (
      // A fresh install has done nothing yet. Saying so beats an empty box, which reads as a
      // panel that failed to load.
      <Stack paddingHorizontal={16} paddingVertical={24}>
        <Text as="p" textAlign="center" textRole="compact" tone="secondary">
          Nothing yet. Activity shows up here as Loomarr acquires titles and builds channels.
        </Text>
      </Stack>
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
              <Stack flexShrink={0} render="span" width={56}>
                <Text as="span" numberOfLines={1} textRole="mono">
                  {/* `at` is unix SECONDS, per the schema's epoch convention; formatRelative takes ms. */}
                  {formatRelative(e.at * 1000, now)}
                </Text>
              </Stack>
              <Stack
                aria-label={e.level}
                backgroundColor={LEVEL_DOT[e.level as keyof typeof LEVEL_DOT] ?? "$actionDisabled"}
                borderRadius={999}
                flexShrink={0}
                height={6}
                render="span"
                role="img"
                width={6}
              />
              <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0} render="span">
                <Text as="span" textRole="compact">
                  {e.text}
                </Text>
              </Stack>
            </Surface>
          </PanelRow>
        ))}
      </ListGroup>
    )}
  </Stack>
);

export { ActivityFeed };
