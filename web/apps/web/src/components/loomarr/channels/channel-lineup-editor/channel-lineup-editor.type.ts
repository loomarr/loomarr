import type { LineupEntryDTO } from "@loomarr/api/models/lineupEntryDTO";

interface ChannelLineupEditorProps {
  channelId: string;
  // The channel's current lineup (ChannelDTO.lineup), in play order. The editor is
  // uncontrolled beyond this: it owns its own commit via useChannelLineup and relies on
  // the caller re-rendering with the server's answer once the page's channel query
  // refetches (the same invalidate the rest of the channel page already does on a
  // `channel` SSE frame or a successful edit).
  lineup: LineupEntryDTO[];
  className?: string;
}

export type { ChannelLineupEditorProps };
