import {
  type ChannelListPress,
  channelListAllowsPress,
  channelListView,
  listRest,
  PRESS_MOVE_PX,
  pinnedShift,
  scrollBounds,
} from "@loomarr/core/channel-list";
import type { LineupChannel, LineupGroupKind } from "@loomarr/core/channel-lineup";
import { Surface, Text } from "@loomarr/design-system";
import { type ComponentRef, Fragment, useEffect, useRef, useState } from "react";
import {
  type GestureResponderEvent,
  type LayoutChangeEvent,
  type NativeScrollEvent,
  type NativeSyntheticEvent,
  Platform,
  Pressable,
  ScrollView,
  View,
  type ViewStyle,
} from "react-native";

import type { ChannelListProps } from "./channel-list.type";
import { ListHead } from "./list-head";
import { ListTarget, type TargetHandle } from "./list-target";

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

// The pinned channel sticks to whichever end of the list it would leave by. Only a browser can stick
// at both ends; a native view is moved by the scroll position instead.
const STICKY =
  Platform.OS === "web"
    ? ({ bottom: 0, position: "sticky", top: 0, zIndex: 2 } as unknown as ViewStyle)
    : ({ zIndex: 2 } as ViewStyle);

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
  const [scrollY, setScrollY] = useState(0);
  // Whether the last input was a key or a remote, which is when the cursor draws its ring.
  const [keys, setKeys] = useState(autoFocus || wholeRows);

  const scroller = useRef<ComponentRef<typeof ScrollView>>(null);
  const rows = useRef(new Map<string, TargetHandle>());
  const tabs = useRef(new Map<LineupGroupKind, TargetHandle>());
  const lastPress = useRef<ChannelListPress | undefined>(undefined);
  const lookTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const looking = useRef<string | undefined>(undefined);
  const rested = useRef("");
  const focusWanted = useRef(autoFocus);

  const pinned = lineup.channels.find((channel) => channel.id === watchedId);
  const pinnedAt = view.finding ? -1 : view.rows.findIndex((channel) => channel.id === watchedId);
  // The watched channel's place down the content: a heading each time a section starts, then rows.
  const before = view.rows.slice(0, pinnedAt < 0 ? view.rows.length : pinnedAt);
  const headingsBefore = view.rows
    .slice(0, pinnedAt < 0 ? view.rows.length : pinnedAt + 1)
    .filter((channel) => view.headings.has(channel.id)).length;
  const room = viewport;
  const anchorY = room + before.length * ROW_HEIGHT + headingsBefore * headingHeight;
  const rest = listRest({
    anchorY,
    pinnedHeight,
    restOffset,
    viewport,
    wholeRows: wholeRows ? ROW_HEIGHT : undefined,
  });
  // Find's results stand at the top of the list.
  const restScroll = view.finding ? room : rest.scroll;
  const notice = view.notice && view.notice !== "no-match" ? NOTICES[view.notice] : undefined;
  const contentHeight =
    2 * room +
    view.rows.length * ROW_HEIGHT +
    view.headings.size * headingHeight +
    (pinned ? pinnedHeight - (pinnedAt < 0 ? 0 : ROW_HEIGHT) : 0);

  // The list re-centres when what it shows changes, and otherwise stays where it was scrolled.
  const restKey = [watchedId, group, view.finding && query, open, viewport, pinnedHeight, restOffset].join();
  useEffect(() => {
    if (!viewport || rested.current === restKey) return;
    rested.current = restKey;
    scroller.current?.scrollTo?.({ animated: false, y: restScroll });
    setScrollY(restScroll);
  });

  // Focus follows the cursor, and only from keys: a pointer never moves the list under itself.
  const cursorKey =
    cursor.kind === "row" ? cursor.channelId : cursor.kind === "tab" ? `tab:${cursor.group}` : "";
  useEffect(() => {
    if (!focusWanted.current || query !== null) return;
    if (cursor.kind === "row") rows.current.get(cursor.channelId)?.focus();
    if (cursor.kind === "tab") tabs.current.get(cursor.group)?.focus();
  }, [cursorKey, query, watchedId, group]);

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
      setKeys(false);
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
    const send: Record<string, Parameters<typeof onEvent>[0] | undefined> = {
      "/": typing || television ? undefined : { query: "", type: "find" },
      ArrowDown: { by: 1, type: "move" },
      ArrowLeft: typing ? undefined : { by: -1, type: "across" },
      ArrowRight: typing ? undefined : { by: 1, type: "across" },
      ArrowUp: { by: -1, type: "move" },
      Backspace: typing ? undefined : { type: query === null ? "close" : "clearFind" },
      Enter: typing ? { type: "choose" } : undefined,
      Escape: { type: query === null ? "close" : "clearFind" },
    };
    focusWanted.current = true;
    setKeys(true);
    const next = send[event.key];
    if (!next) return;
    event.preventDefault();
    onEvent(next);
  };

  const onScroll = (event: NativeSyntheticEvent<NativeScrollEvent>) => {
    const y = event.nativeEvent.contentOffset.y;
    // A different row slides under a resting pointer, so what was looked at is let go.
    look(undefined);
    const bounds = scrollBounds({ contentHeight, height: rest.height, rest: restScroll, room });
    const within = Math.min(bounds.max, Math.max(bounds.min, y));
    if (viewport && within !== y) scroller.current?.scrollTo?.({ animated: false, y: within });
    if (Platform.OS !== "web") setScrollY(within);
  };

  const ring = keys && open;
  const cursorRow = cursor.kind === "row" ? cursor.channelId : undefined;
  const tabStopRow = cursorRow ?? watchedId;
  const rowRef = (channelId: string) => (node: TargetHandle | null) => {
    if (node) rows.current.set(channelId, node);
    else rows.current.delete(channelId);
  };
  const keyFocus = (channelId: string) => () => {
    if (cursorRow !== channelId) onEvent({ channelId, type: "focusRow" });
  };
  // Rows a focus scrolls to stand clear of the pinned channel.
  const clearance = { scrollMarginBottom: pinnedHeight, scrollMarginTop: pinnedHeight } as ViewStyle;
  const spoken = (channel: C) => {
    const { minutesLeft, inBreak } = detail?.(channel) ?? {};
    const left = inBreak ? "in a break" : minutesLeft === undefined ? undefined : `${minutesLeft} min left`;
    return { left, onNow: channel.onNow };
  };

  const rowOf = (channel: C) => {
    const { left, onNow } = spoken(channel);
    const watched = channel.id === watchedId;
    const decade = view.headings.size ? channel.decade : undefined;
    return (
      <ListTarget
        label={[`${channel.number} ${channel.name}`, onNow, left, decade && `${decade}s`, watched && "watching"]
          .filter(Boolean)
          .join(", ")}
        minHeight={ROW_HEIGHT - 4}
        onHoverIn={() => look(channel.id)}
        onHoverOut={() => look(undefined)}
        onKeyFocus={keyFocus(channel.id)}
        onPress={press(channel.id)}
        ringed={ring && cursorRow === channel.id}
        role="option"
        selected={watched}
        style={{ height: ROW_HEIGHT, justifyContent: "center", ...clearance }}
        tabStop={tabStopRow === channel.id}
        targetRef={rowRef(channel.id)}
      >
        <Surface backgroundColor="transparent" borderWidth={0} width={32} alignItems="flex-end">
          <Text textRole="mono" tone="secondary">
            {channel.number}
          </Text>
        </Surface>
        <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
          <Text numberOfLines={1} textRole="cardLabel">
            {channel.name}
          </Text>
          {onNow ? (
            <Text numberOfLines={1} textRole="metadata" tone="secondary">
              {onNow}
            </Text>
          ) : null}
        </Surface>
        {left ? (
          <Text textRole="metadata" tone="secondary">
            {left}
          </Text>
        ) : null}
      </ListTarget>
    );
  };

  const shift =
    Platform.OS === "web"
      ? 0
      : pinnedShift({ anchorY, height: rest.height, pinnedHeight, scroll: scrollY });
  const pinnedSlot = pinned ? (
    <View
      onLayout={(event: LayoutChangeEvent) => {
        const { height } = event.nativeEvent.layout;
        if (height) setPinnedHeight(height);
      }}
      style={[STICKY, shift ? { transform: [{ translateY: shift }] } : null]}
      testID="channel-list-pinned"
    >
      <ListTarget
        label={`${pinned.number} ${pinned.name}, watching`}
        minHeight={ROW_HEIGHT}
        onKeyFocus={view.finding ? undefined : keyFocus(pinned.id)}
        onPress={press(pinned.id)}
        ringed={ring && !view.finding && cursorRow === pinned.id}
        role="option"
        selected={!view.finding}
        tabStop={!view.finding && tabStopRow === pinned.id}
        targetRef={view.finding ? undefined : rowRef(pinned.id)}
      >
        <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
          {renderPinned(pinned)}
        </Surface>
      </ListTarget>
    </View>
  ) : null;

  return (
    <View
      {...(Platform.OS === "web" ? ({ onKeyDown, onPointerMove } as object) : {})}
      style={{ flex: 1, gap: 4, minHeight: 0 }}
    >
      <ListHead
        cursor={cursor}
        group={group}
        onEvent={onEvent}
        query={query}
        ring={ring}
        tabRef={(kind) => (node) => {
          if (node) tabs.current.set(kind, node);
          else tabs.current.delete(kind);
        }}
        television={television}
      />
      <View
        onLayout={(event: LayoutChangeEvent) => setViewport(event.nativeEvent.layout.height)}
        style={{ flex: 1, minHeight: 0 }}
      >
        <ScrollView
          onScroll={onScroll}
          ref={scroller}
          scrollEventThrottle={16}
          showsVerticalScrollIndicator={false}
          style={viewport ? { flexGrow: 0, height: rest.height } : { flex: 1 }}
          testID="channel-list-scroller"
        >
          <View aria-label="Channels" role={"listbox" as never}>
            <View aria-hidden style={{ height: room }} />
            {view.rows.map((channel, index) => (
              <Fragment key={channel.id}>
                {view.headings.has(channel.id) ? (
                  <View aria-hidden style={{ height: headingHeight, justifyContent: "flex-end", paddingLeft: 52 }}>
                    <Text textRole="metadata" tone="secondary">{`${view.headings.get(channel.id)}s`}</Text>
                  </View>
                ) : null}
                {index === pinnedAt ? pinnedSlot : rowOf(channel)}
              </Fragment>
            ))}
            {notice ? (
              <Surface backgroundColor="transparent" borderWidth={0} paddingHorizontal={8} paddingVertical={12}>
                <Text textRole="metadata" tone="secondary">
                  {notice}
                </Text>
              </Surface>
            ) : null}
            {view.finding && !view.rows.length ? (
              <Surface backgroundColor="transparent" borderWidth={0} paddingHorizontal={8} paddingVertical={12}>
                <Text textRole="metadata" tone="secondary">
                  {`No channel or programme matches “${query?.trim()}”.`}
                </Text>
              </Surface>
            ) : null}
            {pinnedAt < 0 ? pinnedSlot : null}
            <View aria-hidden style={{ height: room }} />
          </View>
        </ScrollView>
      </View>
      {view.notice && view.notice !== "empty-recent" ? (
        <Pressable
          accessibilityRole="button"
          onPress={() => onEvent({ type: "showAll" })}
          style={{ justifyContent: "center", minHeight: 44, paddingHorizontal: 8 }}
        >
          <Text textDecorationLine="underline" textRole="cardLabel">
            Show all channels
          </Text>
        </Pressable>
      ) : null}
    </View>
  );
};

export { ChannelList };
