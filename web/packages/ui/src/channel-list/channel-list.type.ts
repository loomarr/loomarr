import type { ChannelLineup, LineupChannel } from "@loomarr/core/channel-lineup";
import type { ChannelListEvent, ChannelListState } from "@loomarr/core/channel-list";
import type { ReactNode, Ref } from "react";

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
  restOffset?: number | undefined;
  /**
   * How far above the bottom of the list the pinned channel's foot rests, for a list whose crossing
   * is measured from below; it takes the place of `restOffset`.
   */
  restFromBottom?: number | undefined;
  /**
   * How far below the top of the whole list, its tabs and find included, the pinned channel rests,
   * for a list whose crossing is set by something beside it; it takes the place of `restOffset`.
   */
  restFromListTop?: number | undefined;
  detail?: ((channel: C) => ChannelListDetail) | undefined;
  layout?: ChannelListLayout | undefined;
  /** Takes focus onto the cursor and draws its ring from the start, as a list opened from a key does. */
  autoFocus?: boolean | undefined;
  /** A pointer resting on another channel, and `undefined` the moment it leaves. */
  onLook?: ((channelId: string | undefined) => void) | undefined;
  /**
   * Where the list was scrolled, kept by the caller across a remount (a change of arrangement). A
   * list mounted showing what it showed when it was kept comes back where it was, not re-centred.
   */
  scrollMemory?: { current: ChannelListScroll | undefined } | undefined;
  ref?: Ref<ChannelListHandle> | undefined;
}

/** What a list was showing, and how far it was scrolled. */
interface ChannelListScroll {
  shows: string;
  scroll: number;
}

interface ChannelListHandle {
  /** Moves focus onto the cursor, for a key that reached the list from outside it. */
  focusCursor: () => void;
}

/**
 * What the list needs of a mounted row or tab. ⚠ Not React Native's View type: Storybook's docgen
 * follows a prop's types into their imports and cannot parse react-native's source.
 */
interface FocusHandle {
  focus: (options?: { preventScroll?: boolean }) => void;
  blur?: () => void;
}

export type {
  ChannelListDetail,
  ChannelListHandle,
  ChannelListLayout,
  ChannelListProps,
  ChannelListScroll,
  FocusHandle,
};
