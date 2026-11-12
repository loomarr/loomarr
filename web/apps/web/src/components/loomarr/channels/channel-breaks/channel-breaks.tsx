import * as channelsApi from "@loomarr/api/endpoints/channels";
import { unwrap } from "@loomarr/api/unwrap";
import { pluralize } from "@loomarr/core/format";
import { Stack, Text } from "@loomarr/design-system";
import { PodTimeline } from "../../filler/pod-timeline";
import type { ChannelBreaksProps } from "./channel-breaks.type";

// ChannelBreaks — the read-only "what plays in this channel's breaks" (§10, §12).
//
// ⚠ **This is the only caller of `GET /v1/channels/{id}/pods`.** Three cache invalidations point
// at its query key, and without this component nothing fills that cache — an endpoint reachable
// only by curl. The endpoint must stay: it is MEMBER-readable, where the draft sandbox
// (`POST …/pods/preview`) is admin-only, so it is the only way a non-admin can see why a channel
// sounds the way it does. An orphaned endpoint is exactly what `/surface-audit` exists to find.
//
// ⚠ Deliberately NOT folded into the Filler section. That section is the admin authoring sandbox
// and is gated accordingly; this is the member-visible answer, on the one screen a member sees.
//
// ⚠ `retry: false` because an install without filler configured answers 501, and this renders
// nothing in that case rather than an error — a channel with no commercials is an ordinary state,
// not a fault. Same treatment the coverage meter gives the same condition.
const ChannelBreaks = ({ channelId, className }: ChannelBreaksProps) => {
  const pods = channelsApi.usePreviewChannelPods(channelId, { query: { retry: false } });
  const pod = unwrap(pods.data);
  const entries = pod?.entries ?? [];

  // Nothing to say yet: still loading, filler is not set up (501), or this channel genuinely has
  // no break pool. All three render as absence rather than as a problem.
  if (entries.length === 0) return null;

  return (
    <Stack className={className} gap="1.5" render="section">
      <h3 className="text-2xs text-muted-foreground font-normal">Between shows</h3>
      <PodTimeline entries={entries} matchLevel={pod?.matchLevel} />
      <Text as="p" textRole="compact" tone="secondary">
        {`${pluralize(entries.length, "clip")} between shows, assembled exactly as the channel builds them.`}
      </Text>
    </Stack>
  );
};

export { ChannelBreaks };
