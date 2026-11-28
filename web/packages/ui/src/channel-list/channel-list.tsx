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
import { decadeTint, RingedPressable, Surface, Text, VisuallyHidden } from "@loomarr/design-system";
import {
  type ComponentProps,
  type ComponentRef,
  Fragment,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from "react";
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

type KeyEvent = {
  key: string;
  nativeEvent?: { isComposing?: boolean };
  preventDefault: () => void;
  target?: { tagName?: string };
};

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
  restOffset: restFromTop = 0,
  restFromBottom,
  detail,
  layout = "pointer",
  autoFocus = false,
  onLook,
  scrollMemory,
  ref,
}: ChannelListProps<C>) => {
  const { cursor, watchedId, group, query, open, television } = state;
  const view = channelListView(state, lineup);
  const wholeRows = layout === "television";
  const headingHeight = wholeRows ? ROW_HEIGHT : HEADING_HEIGHT;

  const [viewport, setViewport] = useState(0);
  const [pinnedHeight, setPinnedHeight] = useState(ROW_HEIGHT * 2);
  const [nativeScroll, setNativeScroll] = useState(0);
  const restOffset =
    restFromBottom === undefined ? restFromTop : Math.max(0, viewport - pinnedHeight - restFromBottom);
  // The row drawn as under a pointer.
  const [hovered, setHovered] = useState<string | undefined>(undefined);

  const scroller = useRef<ComponentRef<typeof ScrollView>>(null);
  const scrolled = useRef(0);
  const rows = useRef(new Map<string, FocusHandle>());
  const tabs = useRef(new Map<LineupGroupKind, FocusHandle>());
  const lastPress = useRef<ChannelListPress | undefined>(undefined);
  const underPointer = useRef<string | undefined>(undefined);
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
  const message =
    view.notice === "no-match"
      ? `No channel or programme matches “${query?.trim()}”.`
      : view.notice && NOTICES[view.notice];
  const anchorY = (pinnedAt < 0 ? y : places.get(watchedId ?? "")) ?? y;
  // A notice stands after the list's last channel, a row high.
  const listEnd = y + (pinnedAt < 0 && pinned ? pinnedHeight : 0);
  const contentHeight = listEnd + (message ? ROW_HEIGHT : 0) + room;
  const rest = listRest({
    anchorY,
    pinnedHeight,
    restOffset,
    viewport,
    wholeRows: wholeRows ? ROW_HEIGHT : undefined,
  });
  // Find's results stand at the top of the list.
  const restScroll = view.finding ? room : rest.scroll;

  // What the list shows, apart from how it is measured.
  const shows = [watchedId, group, view.finding && query, open].join();
  const keep = (scroll: number) => {
    if (scrollMemory) scrollMemory.current = { scroll, shows };
  };
  const scrollTo = (to: number) => {
    scrolled.current = to;
    keep(to);
    scroller.current?.scrollTo?.({ animated: false, y: to });
    if (Platform.OS !== "web") setNativeScroll(to);
  };

  // A list mounted showing what the caller kept comes back where it was, while it still shows that
  // and however its parts measure themselves on the way.
  const kept = useRef(scrollMemory?.current);
  // The list re-centres when what it shows changes, and otherwise stays where it was scrolled.
  const restKey = [shows, viewport, pinnedHeight, restOffset].join();
  useEffect(() => {
    if (!viewport || rested.current === restKey) return;
    rested.current = restKey;
    const back = kept.current?.shows === shows ? kept.current.scroll : undefined;
    if (back === undefined) kept.current = undefined;
    scrollTo(back ?? restScroll);
  });

  // Other rows may now be under a pointer that has not moved, so its highlight is let go.
  const rowsKey = [group, JSON.stringify(query), watchedId].join();
  const drawnKey = useRef(rowsKey);
  useEffect(() => {
    if (drawnKey.current === rowsKey) return;
    drawnKey.current = rowsKey;
    setHovered(undefined);
  });

  // Focus follows the cursor, and only from keys.
  const cursorKey =
    cursor.kind === "row" ? cursor.channelId : cursor.kind === "tab" ? `tab:${cursor.group}` : "";
  // A closed find and an empty one differ here: closing it brings focus back to the cursor.
  const focusKey = [cursorKey, JSON.stringify(query), watchedId, group, viewport].join();
  const focusedKey = useRef("");
  useEffect(() => {
    if (focusedKey.current === focusKey) return;
    focusedKey.current = focusKey;
    if (!focusWanted.current) return;
    if (cursor.kind === "tab") tabs.current.get(cursor.group)?.focus();
    if (cursor.kind !== "row") return;
    // While find is open, focus stays in its query and only the list moves.
    if (query === null) rows.current.get(cursor.channelId)?.focus({ preventScroll: true });
    const rowY = places.get(cursor.channelId);
    // The pinned channel is always in view.
    if (!viewport || rowY === undefined || (!view.finding && cursor.channelId === watchedId)) return;
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

  useImperativeHandle(ref, () => ({
    focusCursor: () => {
      focusWanted.current = true;
      if (cursor.kind === "tab") tabs.current.get(cursor.group)?.focus();
      else if (cursor.kind === "row" && query === null) {
        rows.current.get(cursor.channelId)?.focus({ preventScroll: true });
      }
    },
  }));

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
      setHovered(undefined);
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
    if (Math.hypot(pageX - last.x, pageY - last.y) < PRESS_MOVE_PX) return;
    lastPress.current = undefined;
    setHovered(underPointer.current);
  };

  const onKeyDown = (event: KeyEvent) => {
    // Enter while an input method is building a character finishes the character.
    if (event.key === "Enter" && event.nativeEvent?.isComposing) return;
    const typing = event.target?.tagName === "INPUT";
    const back: ChannelListEvent = { type: "back" };
    const send: Record<string, ChannelListEvent | undefined> = {
      "/": typing || television ? undefined : { query: "", type: "find" },
      ArrowDown: { by: 1, type: "move" },
      ArrowLeft: typing ? undefined : { by: -1, type: "across" },
      ArrowRight: typing ? undefined : { by: 1, type: "across" },
      ArrowUp: { by: -1, type: "move" },
      Backspace: typing ? undefined : back,
      End: typing ? undefined : { to: 1, type: "edge" },
      Enter: typing ? { type: "choose" } : undefined,
      Escape: back,
      Home: typing ? undefined : { to: -1, type: "edge" },
    };
    focusWanted.current = true;
    const next = send[event.key];
    if (!next) return;
    event.preventDefault();
    onEvent(next);
  };
  // React Native Web's text input stops its keys from bubbling, so find's are taken on the way down.
  // Escape alone is raised again by the design system's field, above the input, and is taken there.
  const fromInput = (event: KeyEvent) => event.target?.tagName === "INPUT";
  const keyHandlers = {
    onKeyDown: (event: KeyEvent) => {
      if (!fromInput(event)) onKeyDown(event);
    },
    onKeyDownCapture: (event: KeyEvent) => {
      if (fromInput(event) && event.key !== "Escape") onKeyDown(event);
    },
  };

  const onScroll = (event: NativeSyntheticEvent<NativeScrollEvent>) => {
    const at = event.nativeEvent.contentOffset.y;
    // A different row slides under a resting pointer, so what was looked at is let go.
    look(undefined);
    setHovered(undefined);
    const bounds = scrollBounds({ contentHeight, height: rest.height, rest: restScroll, room });
    const within = Math.min(bounds.max, Math.max(bounds.min, at));
    if (viewport && within !== at) return scrollTo(within);
    scrolled.current = at;
    keep(at);
    if (kept.current) kept.current = { scroll: at, shows };
    if (Platform.OS !== "web") setNativeScroll(at);
  };

  const tabStopRow = cursor.kind === "row" ? cursor.channelId : watchedId;
  const rowRef = (channelId: string) => (node: FocusHandle | null) => {
    if (node) rows.current.set(channelId, node);
    else rows.current.delete(channelId);
  };

  // While find is open, focus and its ring stay in the query, and the row the cursor rests on takes
  // the selected fill, as a combobox's highlighted option does.
  const findCursor =
    query !== null && layout !== "touch" && cursor.kind === "row" ? cursor.channelId : undefined;

  // The cursor resting on the watched channel draws its fill on the pinned slot, behind the
  // caller's drawing, which stands on no plate of its own.
  const pinnedCursor =
    !view.finding && cursor.kind === "row" && cursor.channelId === watchedId ? watchedId : undefined;

  // A row and the pinned channel are options, named by what they show, so each piece of a row's
  // text ends in a space. The highlight is the list's own: under a pointer that a press would
  // answer, on find's cursor, and on the pinned cursor; the ring is the design system's, amber for
  // keys and never for a click. Whether the option is the selected one is said by `selected` alone.
  const target = (channel: C, selected: boolean, isRow: boolean) => ({
    align: "start" as const,
    fill: true,
    highlighted:
      hovered === channel.id || (isRow && findCursor === channel.id) || pinnedCursor === channel.id,
    onFocus: () => {
      if (isRow && (cursor.kind !== "row" || cursor.channelId !== channel.id)) {
        onEvent({ channelId: channel.id, type: "focusRow" });
      }
    },
    onHoverIn: () => {
      underPointer.current = channel.id;
      // A row that slid under a still pointer will not answer a press yet, so it draws nothing.
      if (!lastPress.current) setHovered(channel.id);
      look(channel.id);
    },
    onHoverOut: () => {
      underPointer.current = undefined;
      setHovered(undefined);
      look(undefined);
    },
    // A pointer never takes focus, so the amber ring is the keyboard's and remote's alone and no
    // press scrolls the list under the pointer.
    pointerTakesNoFocus: true,
    onPress: press(channel.id),
    ref: (isRow ? rowRef(channel.id) : undefined) as never,
    role: "option" as const,
    selected,
    size: ROW_HEIGHT - 4,
    tabIndex: (isRow && tabStopRow === channel.id ? 0 : -1) as 0 | -1,
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
        <RingedPressable {...target(channel, watched, true)}>
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
              <Text numberOfLines={1} textAlign="left" textRole="compact" tone="secondary">
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
        </RingedPressable>
      </Bare>
    );
  };

  const shift =
    Platform.OS === "web"
      ? 0
      : pinnedShift({ anchorY, height: rest.height, pinnedHeight, scroll: nativeScroll });
  // Among find's results the watched channel is a row of its own, which is the one marked.
  const listedTwice = view.finding && view.rows.some((channel) => channel.id === watchedId);

  // The pinned channel sticks to whichever end of the list it would leave by. Only a browser can
  // stick at both ends; a native view is moved by the scroll position instead. `sticky` is a web
  // value outside React Native's position type, which the cast gets past.
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
      <RingedPressable {...target(pinned, !listedTwice, !view.finding)}>
        <Bare flex={1} minWidth={0}>
          {renderPinned(pinned)}
        </Bare>
        {listedTwice ? null : <VisuallyHidden>watching</VisuallyHidden>}
      </RingedPressable>
    </Bare>
  ) : null;

  return (
    <Bare
      flex={1}
      gap={4}
      minHeight={0}
      {...(Platform.OS === "web" ? ({ ...keyHandlers, onPointerMove } as object) : {})}
    >
      <ListHead
        cursor={cursor}
        group={group}
        onEvent={onEvent}
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
                      <Text textRole="metadata" tone={decadeTint(view.headings.get(channel.id))}>
                        {`${view.headings.get(channel.id)}s`}
                      </Text>
                    </Bare>
                  ) : null}
                  {index === pinnedAt ? pinnedSlot : rowOf(channel)}
                </Fragment>
              ))}
              {pinnedAt < 0 ? pinnedSlot : null}
              <Bare aria-hidden height={room} />
            </Bare>
            {/* A listbox holds only options, so the notice follows it, drawn up into the room below
                the last channel. */}
            {message ? (
              <Bare height={ROW_HEIGHT} justifyContent="center" marginTop={-room} paddingHorizontal={8}>
                <Text numberOfLines={2} textRole="metadata" tone="secondary">
                  {message}
                </Text>
              </Bare>
            ) : null}
          </ScrollView>
        </Bare>
      </Bare>
      {view.notice && view.notice !== "empty-recent" ? (
        <RingedPressable align="start" fill onPress={() => onEvent({ type: "showAll" })} size={44}>
          <Text textDecorationLine="underline" textRole="cardLabel">
            Show all channels
          </Text>
        </RingedPressable>
      ) : null}
    </Bare>
  );
};

export { ChannelList };
