import type { LineupChannel, LineupGroupKind } from "@loomarr/core/channel-lineup";
import {
  type ChannelListEvent,
  type ChannelListPress,
  channelListAllowsPress,
  channelListView,
  listRest,
  PRESS_MOVE_PX,
  pinnedShift,
  scrollBounds,
  scrollToShow,
} from "@loomarr/core/channel-list";
import { PressableRow, Surface, Text, VisuallyHidden } from "@loomarr/design-system";
import { type ComponentProps, type ComponentRef, Fragment, useEffect, useRef, useState } from "react";
import {
  type GestureResponderEvent,
  type LayoutChangeEvent,
  type NativeScrollEvent,
  type NativeSyntheticEvent,
  Platform,
  ScrollView,
} from "react-native";

import type { ChannelListProps, FocusHandle } from "./channel-list.type";
import { ListHead } from "./list-head";

/** A row's height: at least a finger's, and the same everywhere so a television can rest on whole rows. */
const ROW_HEIGHT = 48;
/** A decade heading's height, except on a television, where it is a row's. */
const HEADING_HEIGHT = 32;
/** How long a pointer rests on a row before it is looking at that channel. */
const LOOK_AFTER_MS = 140;

const NOTICES = {
  "empty-decade": "No channels have a decade yet.",
  "empty-favourites": "No favourites yet.",
  "empty-recent": "Channels you watch collect here.",
} as const;

type KeyEvent = { key: string; preventDefault: () => void; target?: { tagName?: string } };

/** A mouse or finger press carries a place; a key press does not. */
const pointOf = (event: GestureResponderEvent) => {
  // A key press on the web hands over the browser's event itself.
  const native = (event?.nativeEvent ?? event ?? {}) as unknown as {
    type?: string;
    pageX?: number;
    pageY?: number;
    clientX?: number;
    clientY?: number;
  };
  if (native.type?.startsWith("key")) return undefined;
  return { x: native.pageX ?? native.clientX ?? 0, y: native.pageY ?? native.clientY ?? 0 };
};

// A pointer never takes focus, so the amber ring is the keyboard's and remote's alone and no press
// scrolls the list under the pointer.
const keepFocus = (event: { preventDefault: () => void }) => event.preventDefault();

/** A box with no look of its own. */
const Bare = (props: ComponentProps<typeof Surface>) => (
  <Surface backgroundColor="transparent" borderRadius={0} borderWidth={0} {...props} />
);

/**
 * The Cross's channel list (frontend-design §9.5): every channel of a group in one scrollable list,
 * the watched one pinned in it. It draws the state it is given and reports events; it never tunes.
 */
const ChannelList = <C extends LineupChannel>({
  lineup,
  state,
  onEvent,
  renderPinned,
  restOffset,
  detail,
  layout = "pointer",
  autoFocus = false,
  onLook,
}: ChannelListProps<C>) => {
  const { cursor, watchedId, group, query, open, television } = state;
  const view = channelListView(state, lineup);
  const wholeRows = layout === "television";
  const headingHeight = wholeRows ? ROW_HEIGHT : HEADING_HEIGHT;

  const [viewport, setViewport] = useState(0);
  const [pinnedHeight, setPinnedHeight] = useState(ROW_HEIGHT * 2);
  const [nativeScroll, setNativeScroll] = useState(0);

  const scroller = useRef<ComponentRef<typeof ScrollView>>(null);
  const scrolled = useRef(0);
  const rows = useRef(new Map<string, FocusHandle>());
  const tabs = useRef(new Map<LineupGroupKind, FocusHandle>());
  const lastPress = useRef<ChannelListPress | undefined>(undefined);
  const lookTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const looking = useRef<string | undefined>(undefined);
  const rested = useRef("");
  // Whether the last input was a key or a remote, which is when focus follows the cursor.
  const focusWanted = useRef(autoFocus || wholeRows);

  const pinned = lineup.channels.find((channel) => channel.id === watchedId);
  const pinnedAt = view.finding ? -1 : view.rows.findIndex((channel) => channel.id === watchedId);

  // Each row's place down the content, below the room above the first: a heading, then its rows.
  const room = viewport;
  const places = new Map<string, number>();
  let y = room;
  view.rows.forEach((channel, index) => {
    if (view.headings.has(channel.id)) y += headingHeight;
    places.set(channel.id, y);
    y += index === pinnedAt ? pinnedHeight : ROW_HEIGHT;
  });
  const anchorY = (pinnedAt < 0 ? y : places.get(watchedId ?? "")) ?? y;
  const contentHeight = y + (pinnedAt < 0 && pinned ? pinnedHeight : 0) + room;
  const rest = listRest({
    anchorY,
    pinnedHeight,
    restOffset,
    viewport,
    wholeRows: wholeRows ? ROW_HEIGHT : undefined,
  });
  // Find's results stand at the top of the list.
  const restScroll = view.finding ? room : rest.scroll;

  const scrollTo = (to: number) => {
    scrolled.current = to;
    scroller.current?.scrollTo?.({ animated: false, y: to });
    if (Platform.OS !== "web") setNativeScroll(to);
  };

  // The list re-centres when what it shows changes, and otherwise stays where it was scrolled.
  const restKey = [watchedId, group, view.finding && query, open, viewport, pinnedHeight, restOffset].join();
  useEffect(() => {
    if (!viewport || rested.current === restKey) return;
    rested.current = restKey;
    scrollTo(restScroll);
  });

  // Focus follows the cursor, and only from keys.
  const cursorKey =
    cursor.kind === "row" ? cursor.channelId : cursor.kind === "tab" ? `tab:${cursor.group}` : "";
  const focusKey = [cursorKey, query, watchedId, group, viewport].join();
  const focusedKey = useRef("");
  useEffect(() => {
    if (focusedKey.current === focusKey) return;
    focusedKey.current = focusKey;
    if (!focusWanted.current || query !== null) return;
    if (cursor.kind === "tab") tabs.current.get(cursor.group)?.focus();
    if (cursor.kind !== "row") return;
    rows.current.get(cursor.channelId)?.focus({ preventScroll: true });
    const rowY = places.get(cursor.channelId);
    if (!viewport || rowY === undefined || cursor.channelId === watchedId) return;
    const to = scrollToShow({
      anchorY,
      height: rest.height,
      pinnedHeight,
      rowHeight: ROW_HEIGHT,
      rowY,
      scroll: scrolled.current,
    });
    if (to !== scrolled.current) scrollTo(to);
  });

  const look = (channelId: string | undefined) => {
    clearTimeout(lookTimer.current);
    if (channelId === undefined || channelId === watchedId) {
      if (looking.current !== undefined) onLook?.(undefined);
      looking.current = undefined;
      return;
    }
    // A pointer that has not moved since a press is looking at nothing.
    if (layout !== "pointer" || lastPress.current) return;
    lookTimer.current = setTimeout(() => {
      looking.current = channelId;
      onLook?.(channelId);
    }, LOOK_AFTER_MS);
  };
  useEffect(() => () => clearTimeout(lookTimer.current), []);

  const press = (channelId: string) => (event: GestureResponderEvent) => {
    const point = pointOf(event);
    if (point) {
      const next = { ...point, at: Date.now(), touch: layout === "touch" };
      if (!channelListAllowsPress(lastPress.current, next)) return;
      lastPress.current = next;
      focusWanted.current = false;
      // The ring a key left behind goes with the keys.
      if (cursor.kind === "row") rows.current.get(cursor.channelId)?.blur?.();
    }
    look(undefined);
    onEvent({ channelId, type: "choose" });
  };

  const onPointerMove = (event: { nativeEvent: { pageX?: number; pageY?: number } }) => {
    const last = lastPress.current;
    if (!last) return;
    const { pageX = 0, pageY = 0 } = event.nativeEvent;
    if (Math.hypot(pageX - last.x, pageY - last.y) >= PRESS_MOVE_PX) lastPress.current = undefined;
  };

  const onKeyDown = (event: KeyEvent) => {
    const typing = event.target?.tagName === "INPUT";
    const back: ChannelListEvent = { type: query === null ? "close" : "clearFind" };
    const send: Record<string, ChannelListEvent | undefined> = {
      "/": typing || television ? undefined : { query: "", type: "find" },
      ArrowDown: { by: 1, type: "move" },
      ArrowLeft: typing ? undefined : { by: -1, type: "across" },
      ArrowRight: typing ? undefined : { by: 1, type: "across" },
      ArrowUp: { by: -1, type: "move" },
      Backspace: typing ? undefined : back,
      Enter: typing ? { type: "choose" } : undefined,
      Escape: back,
    };
    focusWanted.current = true;
    const next = send[event.key];
    if (!next) return;
    event.preventDefault();
    onEvent(next);
  };

  const onScroll = (event: NativeSyntheticEvent<NativeScrollEvent>) => {
    const at = event.nativeEvent.contentOffset.y;
    // A different row slides under a resting pointer, so what was looked at is let go.
    look(undefined);
    const bounds = scrollBounds({ contentHeight, height: rest.height, rest: restScroll, room });
    const within = Math.min(bounds.max, Math.max(bounds.min, at));
    if (viewport && within !== at) return scrollTo(within);
    scrolled.current = at;
    if (Platform.OS !== "web") setNativeScroll(at);
  };

  const tabStopRow = cursor.kind === "row" ? cursor.channelId : watchedId;
  const rowRef = (channelId: string) => (node: FocusHandle | null) => {
    if (node) rows.current.set(channelId, node);
    else rows.current.delete(channelId);
  };

  // A row or the pinned channel is named by what it shows, so each piece of text ends in a space.
  const target = (channel: C, selected: boolean, isRow: boolean) => ({
    onFocus: () => {
      if (isRow && (cursor.kind !== "row" || cursor.channelId !== channel.id)) {
        onEvent({ channelId: channel.id, type: "focusRow" });
      }
    },
    onHoverIn: () => look(channel.id),
    onHoverOut: () => look(undefined),
    onMouseDown: keepFocus,
    onPress: press(channel.id),
    ref: (isRow ? rowRef(channel.id) : undefined) as never,
    ...({
      "aria-selected": selected,
      role: "option",
      tabIndex: isRow && tabStopRow === channel.id ? 0 : -1,
    } as object),
  });

  const rowOf = (channel: C) => {
    const { minutesLeft, inBreak } = detail?.(channel) ?? {};
    const left = inBreak ? "in a break" : minutesLeft === undefined ? undefined : `${minutesLeft} min left`;
    const watched = channel.id === watchedId;
    const decade = view.headings.size ? channel.decade : undefined;
    // What is not drawn but is said: the heading a row stands under, and that it is on screen.
    const unseen = [decade && `${decade}s`, watched && "watching"].filter(Boolean).join(" ");
    return (
      <Bare height={ROW_HEIGHT} justifyContent="center">
        <PressableRow inset={{ x: 8, y: 4 }} {...target(channel, watched, true)}>
          <Bare alignItems="flex-end" width={32}>
            <Text textRole="mono" tone="secondary">
              {channel.number}{" "}
            </Text>
          </Bare>
          <Bare flex={1} minWidth={0}>
            <Text numberOfLines={1} textAlign="left" textRole="cardLabel">
              {channel.name}{" "}
            </Text>
            {channel.onNow ? (
              <Text numberOfLines={1} textAlign="left" textRole="metadata" tone="secondary">
                {channel.onNow}{" "}
              </Text>
            ) : null}
          </Bare>
          {left ? (
            <Text textRole="metadata" tone="secondary">
              {left}{" "}
            </Text>
          ) : null}
          {unseen ? <VisuallyHidden>{unseen}</VisuallyHidden> : null}
        </PressableRow>
      </Bare>
    );
  };

  const shift =
    Platform.OS === "web"
      ? 0
      : pinnedShift({ anchorY, height: rest.height, pinnedHeight, scroll: nativeScroll });
  // The pinned channel sticks to whichever end of the list it would leave by. Only a browser can
  // stick at both ends; a native view is moved by the scroll position instead.
  const pinnedSlot = pinned ? (
    <Bare
      bottom={0}
      onLayout={(event: LayoutChangeEvent) => {
        const { height } = event.nativeEvent.layout;
        if (height) setPinnedHeight(height);
      }}
      position={(Platform.OS === "web" ? "sticky" : "relative") as "relative"}
      testID="channel-list-pinned"
      top={0}
      y={shift}
      zIndex={2}
    >
      <PressableRow inset={{ x: 2, y: 2 }} {...target(pinned, !view.finding, !view.finding)}>
        <Bare flex={1} minWidth={0}>
          {renderPinned(pinned)}
        </Bare>
        <VisuallyHidden>watching</VisuallyHidden>
      </PressableRow>
    </Bare>
  ) : null;

  const message =
    view.notice === "no-match"
      ? `No channel or programme matches “${query?.trim()}”.`
      : view.notice && NOTICES[view.notice];

  return (
    <Bare
      flex={1}
      gap={4}
      minHeight={0}
      {...(Platform.OS === "web" ? ({ onKeyDown, onPointerMove } as object) : {})}
    >
      <ListHead
        cursor={cursor}
        group={group}
        onEvent={onEvent}
        onMouseDown={keepFocus}
        query={query}
        tabRef={(kind) => (node) => {
          if (node) tabs.current.set(kind, node);
          else tabs.current.delete(kind);
        }}
        television={television}
      />
      <Bare
        flex={1}
        minHeight={0}
        onLayout={(event: LayoutChangeEvent) => setViewport(event.nativeEvent.layout.height)}
      >
        <Bare height={viewport ? rest.height : "100%"}>
          <ScrollView
            onScroll={onScroll}
            ref={scroller}
            scrollEventThrottle={16}
            showsVerticalScrollIndicator={false}
            testID="channel-list-scroller"
          >
            <Bare aria-label="Channels" role={"listbox" as never}>
              <Bare aria-hidden height={room} />
              {view.rows.map((channel, index) => (
                <Fragment key={channel.id}>
                  {view.headings.has(channel.id) ? (
                    <Bare aria-hidden height={headingHeight} justifyContent="flex-end" paddingLeft={52}>
                      <Text textRole="metadata" tone="secondary">{`${view.headings.get(channel.id)}s`}</Text>
                    </Bare>
                  ) : null}
                  {index === pinnedAt ? pinnedSlot : rowOf(channel)}
                </Fragment>
              ))}
              {message ? (
                <Bare paddingHorizontal={8} paddingVertical={12}>
                  <Text textRole="metadata" tone="secondary">
                    {message}
                  </Text>
                </Bare>
              ) : null}
              {pinnedAt < 0 ? pinnedSlot : null}
              <Bare aria-hidden height={room} />
            </Bare>
          </ScrollView>
        </Bare>
      </Bare>
      {view.notice && view.notice !== "empty-recent" ? (
        <PressableRow inset={{ x: 8, y: 12 }} onPress={() => onEvent({ type: "showAll" })}>
          <Text textDecorationLine="underline" textRole="cardLabel">
            Show all channels
          </Text>
        </PressableRow>
      ) : null}
    </Bare>
  );
};

export { ChannelList };
