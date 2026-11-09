import type { ChannelIdeaDTO } from "@loomarr/api/models/channelIdeaDTO";

interface ChannelIdeasProps {
  // Nothing is on the air yet: the subtitle invites a first channel, and an idea's reason counts
  // library titles instead of saying none are on a channel (none are).
  empty: boolean;
  // Wrap the section in the page inset, for a page that has no padded container of its own.
  inset?: boolean;
  nowMs: number;
}

interface IdeaCardProps {
  idea: ChannelIdeaDTO;
  empty: boolean;
  nowMs: number;
  // True while this card's request or hide is in flight, so a double click can't send two.
  busy: boolean;
  onRequest: () => void;
  onHide: () => void;
}

export type { ChannelIdeasProps, IdeaCardProps };
