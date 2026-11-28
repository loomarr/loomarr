import type { ChannelLineup, LineupChannel } from "@loomarr/core/channel-lineup";
import type { ChannelListEvent, ChannelListState } from "@loomarr/core/channel-list";
import type { ReactNode } from "react";

/** How the list is used: a pointer in a window, a finger, or a remote. */
type ChannelListLayout = "pointer" | "touch" | "television";

/** What a row says about the time left, beside the title the channel carries as `onNow`. */
interface ChannelListDetail {
  minutesLeft?: number | undefined;
  inBreak?: boolean | undefined;
}

interface ChannelListProps<C extends LineupChannel> {
  lineup: ChannelLineup<C>;
  /** The model's state; the caller runs `channelListReduce` over `onEvent` and tunes what it asks. */
  state: ChannelListState;
  onEvent: (event: ChannelListEvent) => void;
  /** The watched channel's drawing, pinned in the list. It holds nothing that takes focus. */
  renderPinned: (channel: C) => ReactNode;
  /** How far below the top of the list the pinned channel rests. */
  restOffset: number;
  detail?: ((channel: C) => ChannelListDetail) | undefined;
  layout?: ChannelListLayout | undefined;
  /** Takes focus onto the cursor and draws its ring from the start, as a list opened from a key does. */
  autoFocus?: boolean | undefined;
  /** A pointer resting on another channel, and `undefined` the moment it leaves. */
  onLook?: ((channelId: string | undefined) => void) | undefined;
}

/**
 * What the list needs of a mounted row or tab. ⚠ Not React Native's View type: Storybook's docgen
 * follows a prop's types into their imports and cannot parse react-native's source.
 */
interface FocusHandle {
  focus: (options?: { preventScroll?: boolean }) => void;
  blur?: () => void;
}

export type { ChannelListDetail, ChannelListLayout, ChannelListProps, FocusHandle };
