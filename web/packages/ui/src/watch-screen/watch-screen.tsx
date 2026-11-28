import type { LineupChannel, LineupGroupKind } from "@loomarr/core/channel-lineup";
import {
  Icon,
  icons,
  Layer,
  RingedPressable,
  Surface,
  Text,
  ThemeScope,
  useReducedMotionPreference,
  VisuallyHidden,
} from "@loomarr/design-system";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { Animated, type LayoutChangeEvent, Platform } from "react-native";

import { ChannelList, type ChannelListHandle, type ChannelListScroll } from "../channel-list";
import { FilmStrip, type FilmStripHandle, type FilmStripLook } from "../film-strip";
import { FirstRunHint } from "../first-run-hint";
import { LowerThird, type LowerThirdState } from "../lower-third";
import { ProgrammeFacts } from "../programme-facts";
import { QuietRow, type QuietRowHandle, type QuietRowKey } from "../quiet-row";
import { TuneBars, TuneStatic } from "../tune-static";
import { WatchScrim } from "../watch-scrim";
import type { WatchScreenProps } from "./watch-screen.type";

// The full-screen arrangement (design/watch, `full`): the list's column stands in from the leading
// edge, the lower third rests on the crossing, the quiet row sits in the trailing corner.
const EDGE = 64;
const LIST_WIDTH = 440;
const LIST_TOP = 28;
const LIST_FOOT = 20;
/** The lower third's foot, above the bottom of the screen; the list rests its pinned channel there. */
const CROSSING = 148;
const ROW_FOOT = 40;
const HINT_TOP = 68;
/** The menu stands over the quiet row. */
const MENU_FOOT = 96;
/** How long the controls take to rest or wake. */
const FADE_MS = 200;
/** Pointer movement wakes the screen; while awake it is reported at most this often. */
const MOVE_EVERY_MS = 500;

// The window arrangement (frontend-design §9.7): the list's column beside the picture, the line,
// the film strip and the facts under it.
const WINDOW_TOP = 28;
const WINDOW_EDGE = 48;
const WINDOW_FOOT = 32;
const WINDOW_GAP = 48;
const WINDOW_LIST_WIDTH = 400;
/** The line under the picture: the label and the quiet row. */
const LINE_HEIGHT = 36;
const FACTS_GAP = 14;
const FACTS_HEIGHT = 92;
/** Before the room is measured: the line, the strip with its words, and the facts under it. */
const GUESSED_UNDER = LINE_HEIGHT + 138 + FACTS_GAP + FACTS_HEIGHT;
/** How far along its day the strip draws from what is on now. */
const STRIP_SPAN_MS = 6 * 60 * 60_000;
/**
 * Narrower than this the window stacks as an upright phone does: the list, its gap and the margins
 * leave the picture 356 px across here, and any narrower it would be smaller than the list.
 */
const NARROW_BELOW = 900;
const NARROW_EDGE = 16;
const PICTURE_RATIO = 16 / 9;

const GROUP_NAMES: Record<LineupGroupKind, string> = {
  all: "All",
  decade: "By decade",
  favourites: "Favourites",
  recent: "Recent",
};

const QUIET_ROW_MENUS = new Set<string>(["audio", "more", "subtitles"]);

/** An opacity that follows `shown`, fading unless motion is reduced. */
const useShown = (shown: boolean, reducedMotion: boolean | undefined) => {
  const still = useReducedMotionPreference(reducedMotion) !== false;
  const level = useRef(new Animated.Value(shown ? 1 : 0)).current;
  useEffect(() => {
    if (still) {
      level.setValue(shown ? 1 : 0);
      return undefined;
    }
    const fade = Animated.timing(level, {
      duration: FADE_MS,
      toValue: shown ? 1 : 0,
      useNativeDriver: false,
    });
    fade.start();
    return () => fade.stop();
  }, [level, shown, still]);
  return level;
};

/** The web's pointer events, which React Native's view types leave out. */
const web = (handlers: object): object => (Platform.OS === "web" ? handlers : {});

type Size = { height: number; width: number };
type Rect = Size & { x: number; y: number };
/** Where the one picture layer stands: the whole screen, or a box an arrangement measured. */
type PictureAt = { fill: true } | { height: number; left: number; top: number; width: number };
const sizeOf = (event: LayoutChangeEvent): Size => {
  const { height, width } = event.nativeEvent.layout;
  return { height, width };
};
/** The browser window's size, before anything is measured; nothing off the web. */
const browserWindow = (): Size => {
  const page = (
    globalThis as { document?: { documentElement?: { clientHeight: number; clientWidth: number } } }
  ).document?.documentElement;
  return { height: page?.clientHeight ?? 0, width: page?.clientWidth ?? 0 };
};
const rectOf = (event: LayoutChangeEvent): Rect => {
  const { height, width, x, y } = event.nativeEvent.layout;
  return { height, width, x, y };
};

/**
 * The watching screen of The Cross (frontend-design §9.1, target): the picture with the tune, the
 * lower third, the quiet row, the first-run hint and the channel list. At full screen they are drawn
 * over the picture; in a window the list stands beside it and the film strip under it (§9.7). It
 * draws what `useWatchScreen` says and reports what happened; the caller does what it asks.
 */
const WatchScreen = <C extends LineupChannel>({
  back,
  cover,
  detail,
  holdSnow,
  layout,
  model,
  muted,
  picture,
  programme,
  reducedMotion,
  renderMenu,
  schedule,
  tune,
}: WatchScreenProps<C>) => {
  const { lineup, send, state, view } = model;
  const inWindow = layout === "window";
  const quietRow = useRef<QuietRowHandle>(null);
  const list = useRef<ChannelListHandle>(null);
  const strip = useRef<FilmStripHandle>(null);
  const lastMove = useRef(Number.NEGATIVE_INFINITY);
  const shown = useShown(view.awake, reducedMotion);
  const watched = lineup.channels.find((channel) => channel.id === state.watchedId);

  // The window's measurements: the screen, the picture's room, and where the line under it starts.
  // Until the screen is measured, the browser's window is the best guess at its size, so a narrow
  // window never draws the columns first.
  const [measured, setMeasured] = useState<Size | undefined>(undefined);
  const box = measured ?? browserWindow();
  const [room, setRoom] = useState<Rect | undefined>(undefined);
  const [lineTop, setLineTop] = useState(0);
  const [lineHeight, setLineHeight] = useState(LINE_HEIGHT);
  const [listTop, setListTop] = useState(0);
  const narrow = inWindow && box.width > 0 && box.width < NARROW_BELOW;
  // Crossing the breakpoint draws the list anew, so find closes with it, as a change of layout
  // closes the list's other layers.
  const wasNarrow = useRef(narrow);
  useEffect(() => {
    if (wasNarrow.current === narrow) return;
    wasNarrow.current = narrow;
    if (state.list.query !== null) send({ event: { type: "clearFind" }, type: "list" });
  }, [narrow, send, state.list.query]);
  // Each of the window's lists keeps its scroll while the other arrangement is drawn.
  const wideScroll = useRef<ChannelListScroll | undefined>(undefined);
  const narrowScroll = useRef<ChannelListScroll | undefined>(undefined);
  // The programme a pointer or focus is on in the strip, for the channel it was on.
  const [pointed, setPointed] = useState<{ channelId: string; look: FilmStripLook } | undefined>(undefined);

  // The model refuses a pause while there is no picture to hold.
  const showing = cover ? "covered" : tune.cover === "cold" ? "tuning" : "playing";
  useEffect(() => {
    send({ picture: showing, type: "picture" });
  }, [send, showing]);

  // The one gate for moving focus because a control went away or a layer closed: only after a
  // key or a remote, by the hook's one note of the last input on the page. After a pointer press
  // focus stays where it is, on the page.
  const { keyed } = model;
  const handFocus = (move: () => void) => {
    if (keyed()) move();
  };

  // The one pointer rule for the screen's chrome: at full screen the foot, the lower third and the
  // quiet row; in a window every region drawn beside the picture, the list's header and the line,
  // strip and facts under the picture. A press there takes focus off whichever of their controls
  // holds it, so no keyboard ring outlives a click. The list, the menu and the cover are not part
  // of it.
  type Part = { contains?: (node: unknown) => boolean } | null;
  const footThird = useRef<Part>(null);
  const footRow = useRef<Part>(null);
  const windowHead = useRef<Part>(null);
  const windowLine = useRef<Part>(null);
  const windowRoot = useRef<Part>(null);
  const activeElement = () =>
    (globalThis as { document?: { activeElement?: unknown } }).document?.activeElement;
  const blurIn = (parts: readonly Part[]) => {
    const active = activeElement();
    if (active && parts.some((part) => part?.contains?.(active))) (active as { blur?: () => void }).blur?.();
  };
  const pressChrome = () =>
    blurIn(inWindow ? [windowHead.current, windowLine.current] : [footThird.current, footRow.current]);
  const partRef = (part: { current: Part }) => (node: unknown) => {
    part.current = node as Part;
  };

  // Focus goes where the model hands it: onto the list's cursor, back to Channels, back to the
  // control whose menu closed, and in a window off every control to the picture, or into the strip.
  const handOff = state.handOff;
  // biome-ignore lint/correctness/useExhaustiveDependencies: each hand-off moves focus once
  useEffect(() => {
    if (!handOff) return;
    if (handOff.to === "list") handFocus(() => list.current?.focusCursor());
    else if (handOff.to === "channels") handFocus(() => quietRow.current?.focusHead());
    else if (handOff.to === "picture") handFocus(() => blurIn([windowRoot.current]));
    else if (handOff.to === "strip") {
      const { by } = handOff;
      handFocus(() => strip.current?.focusFromNow(by));
    } else {
      const to = handOff.to;
      handFocus(() => quietRow.current?.focusControl(to));
    }
  }, [handOff?.seq]);

  const thirdState = (channel: C): LowerThirdState => {
    if (view.digits) {
      const { target, typed } = view.digits;
      return {
        digits: typed,
        kind: "typing",
        ...(target
          ? { target: { name: target.name, ...(target.onNow ? { onNow: target.onNow } : {}) } }
          : {}),
      };
    }
    if (tune.cover === "cold") return { kind: "tuning" };
    const on = programme(channel);
    return on.kind === "on" ? { ...on, behindSeconds: view.behindSeconds, paused: view.paused } : on;
  };

  const third = (channel: C, controls: boolean) => (
    <LowerThird
      channelName={channel.name}
      channelNumber={channel.number}
      controls={controls}
      device={inWindow ? "window" : "full"}
      onGoLive={() => send({ control: "goLive", type: "press" })}
      onGoLiveDropped={() => handFocus(() => quietRow.current?.focusHead())}
      onGoLiveFocus={(focused) => send({ control: focused ? "goLive" : undefined, type: "focus" })}
      ownInk={false}
      state={thirdState(channel)}
      {...(channel.decade === undefined ? {} : { decade: channel.decade })}
    />
  );

  // While find is open its field keeps focus, so the result under its cursor is said aloud.
  const { cursor, query } = state.list;
  const found =
    query?.trim() && cursor.kind === "row"
      ? lineup.channels.find((each) => each.id === cursor.channelId)
      : undefined;
  const announcement = !query?.trim()
    ? ""
    : found
      ? [`${found.number} ${found.name}`, found.onNow].filter(Boolean).join(", ")
      : "No channel or programme matches";

  // The lower third's element arrives, and does two things: it joins the foot, for the pointer
  // rule, and, since it comes back under a pointer that has not moved when the list closes with
  // no pointer-enter, it asks whether the pointer is already over it.
  const thirdArrived = useCallback(
    (node: unknown) => {
      footThird.current = node as Part;
      const element = node as { matches?: (selector: string) => boolean } | null;
      if (element?.matches?.(":hover")) send({ held: true, region: "lowerThird", type: "pointerHold" });
    },
    [send],
  );

  // The pointer resting on the controls holds the screen awake.
  const holdOn = (region: "lowerThird" | "quietRow") =>
    web({
      onPointerEnter: () => send({ held: true, region, type: "pointerHold" }),
      onPointerLeave: () => send({ held: false, region, type: "pointerHold" }),
    });
  const onPointerMove = () => {
    const at = Date.now();
    if (view.awake && at - lastMove.current < MOVE_EVERY_MS) return;
    lastMove.current = at;
    send({ type: "pointerMove" });
  };
  const openKey =
    view.menu && QUIET_ROW_MENUS.has(view.menu.name) ? (view.menu.name as QuietRowKey) : undefined;

  // The picture, the tune's static over it, a cover, and the catcher for clicks on it, all in the
  // one picture layer: at full screen it fills the screen; in a window it is the picture's box, so
  // the static covers the picture alone.
  const pictureStack = (
    <>
      <Layer fill>{picture}</Layer>
      <TuneStatic
        cover={tune.cover}
        {...(holdSnow ? { holdSnow } : {})}
        {...(reducedMotion === undefined ? {} : { reducedMotion })}
      />
      {cover ? <Layer fill>{cover}</Layer> : null}
      {/* A click pauses or closes what is open; a double click is full screen. Space does the same
          from the keyboard, so the catcher is not a stop of its own. */}
      <Layer aria-hidden fill testID="watch-picture">
        <Surface
          backgroundColor="transparent"
          borderWidth={0}
          flex={1}
          onPress={() => send({ type: "pictureClick" })}
          {...web({ onDoubleClick: () => send({ type: "pictureDoubleClick" }) })}
        />
      </Layer>
    </>
  );

  const bars = (
    <TuneBars bars={tune.bars} run={tune.run} {...(reducedMotion === undefined ? {} : { reducedMotion })} />
  );

  const menu =
    view.menu && renderMenu
      ? renderMenu({
          close: () => send({ fromKey: keyed(), type: "closeMenu" }),
          lastChannel: view.lastChannel,
          name: view.menu.name,
          openPage: (page) => send({ page, type: "menuPage" }),
          page: view.menu.page,
        })
      : null;

  const liveRegion = (
    <VisuallyHidden>
      <Text accessibilityLiveRegion="polite" textRole="body">
        {announcement}
      </Text>
    </VisuallyHidden>
  );

  // Where the picture layer stands, and the chrome the arrangement draws around and over it.
  let pictureAt: PictureAt = { fill: true };
  let chrome: ReactNode = null;

  if (!inWindow) {
    chrome = (
      <>
        <Layer bottom={0} left={0} opacity={shown} pointerEvents="none" right={0} top={0}>
          <WatchScrim edge="foot" />
        </Layer>
        {view.listOpen ? <WatchScrim edge="side" /> : null}

        <ThemeScope theme="dark">
          {view.hint ? (
            <Layer centred opacity={shown} top={HINT_TOP}>
              <FirstRunHint device="full" />
            </Layer>
          ) : null}

          {watched && !view.listOpen ? (
            <Layer
              bottom={CROSSING}
              left={EDGE}
              opacity={shown}
              testID="watch-lower-third"
              {...holdOn("lowerThird")}
            >
              <Surface
                backgroundColor="transparent"
                borderWidth={0}
                ref={thirdArrived}
                {...web({ onMouseDown: pressChrome })}
              >
                {third(watched, true)}
              </Surface>
            </Layer>
          ) : null}

          {view.listOpen ? (
            <Layer bottom={LIST_FOOT} left={EDGE} testID="watch-list" top={LIST_TOP} width={LIST_WIDTH}>
              <ChannelList
                autoFocus
                detail={detail}
                lineup={lineup}
                onEvent={(event) => send({ event, fromKey: keyed(), type: "list" })}
                ref={list}
                renderPinned={(channel) => third(channel, false)}
                restFromBottom={CROSSING - LIST_FOOT}
                state={state.list}
              />
            </Layer>
          ) : null}

          <Layer
            bottom={ROW_FOOT}
            opacity={shown}
            right={EDGE}
            testID="watch-quiet-row"
            {...holdOn("quietRow")}
          >
            <Surface
              backgroundColor="transparent"
              borderWidth={0}
              ref={partRef(footRow)}
              {...web({ onMouseDown: pressChrome })}
            >
              <QuietRow
                device="full"
                group={GROUP_NAMES[state.list.group]}
                listOpen={view.listOpen}
                muted={muted ?? false}
                onFocusDropped={() => handFocus(() => quietRow.current?.focusHead())}
                onFocusKey={(key) => send({ control: key, type: "focus" })}
                onPress={(key) => send({ control: key, type: "press" })}
                ownInk={false}
                paused={view.paused}
                ref={quietRow}
                {...(openKey ? { openKey } : {})}
              />
            </Surface>
          </Layer>

          {menu ? (
            <Layer bottom={MENU_FOOT} right={EDGE} testID="watch-menu">
              {menu}
            </Layer>
          ) : null}
        </ThemeScope>

        {/* The seven bars run along the crossing. */}
        <Layer bottom={CROSSING - 8} left={0} pointerEvents="none" right={0}>
          {bars}
        </Layer>
      </>
    );
  } else {
    const quietRowOf = (behindLive: boolean) => (
      <QuietRow
        behindLive={behindLive}
        device="window"
        muted={muted ?? false}
        onFocusDropped={() => handFocus(() => quietRow.current?.focusHead())}
        onFocusKey={(key) => send({ control: key, type: "focus" })}
        onPress={(key) => send({ control: key, type: "press" })}
        ownInk={false}
        paused={view.paused}
        ref={quietRow}
        {...(openKey ? { openKey } : {})}
      />
    );

    // A pointer's target in the columns; the narrow stack is what a phone's browser gets, so there
    // its targets are a finger's.
    const target = narrow ? 44 : 36;
    const header = (
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap={8}
        justifyContent={back ? "space-between" : "flex-end"}
        ref={partRef(windowHead)}
        testID="watch-window-head"
        {...web({ onMouseDown: pressChrome })}
      >
        {back ? (
          <RingedPressable
            accessibilityLabel={back.label}
            onPress={back.onPress}
            pointerTakesNoFocus
            size={target}
          >
            <Icon decorative glyph={icons.back} size="control" tone="content" />
            <Text numberOfLines={1} textRole="label" tone="primary">
              {back.label}
            </Text>
          </RingedPressable>
        ) : null}
        <RingedPressable
          accessibilityLabel="Guide"
          onPress={() => send({ control: "guide", type: "press" })}
          pointerTakesNoFocus
          size={target}
        >
          <Icon decorative glyph={icons.guide} size="control" tone="content" />
          <Text numberOfLines={1} textRole="label" tone="primary">
            Guide
          </Text>
        </RingedPressable>
      </Surface>
    );

    const listOf = (props: {
      onLook?: (channelId: string | undefined) => void;
      renderPinned: (channel: C) => ReactNode;
      restFromListTop?: number;
      scrollMemory: { current: ChannelListScroll | undefined };
    }) => (
      <Surface
        backgroundColor="transparent"
        borderWidth={0}
        flex={1}
        minHeight={0}
        onLayout={(event: LayoutChangeEvent) => setListTop(event.nativeEvent.layout.y)}
        testID="watch-list"
      >
        <ChannelList
          detail={detail}
          lineup={lineup}
          onEvent={(event) => send({ event, fromKey: keyed(), type: "list" })}
          ref={list}
          state={state.list}
          {...props}
        />
      </Surface>
    );

    const hint = view.hint ? <FirstRunHint device="window" /> : null;
    // A menu stands over the quiet row, which ends the line under the picture.
    const menuFoot = box.height ? box.height - (WINDOW_TOP + lineTop) + 8 : MENU_FOOT;

    if (narrow) {
      const pictureWidth = Math.max(0, box.width - NARROW_EDGE * 2);
      pictureAt = {
        height: pictureWidth / PICTURE_RATIO,
        left: NARROW_EDGE,
        top: NARROW_EDGE,
        width: pictureWidth,
      };
      chrome = (
        <>
          <ThemeScope theme="dark">
            <Surface
              backgroundColor="surfaceCanvas"
              borderRadius={0}
              borderWidth={0}
              flex={1}
              gap={12}
              padding={NARROW_EDGE}
              ref={partRef(windowRoot)}
              testID="watch-window-narrow"
            >
              {/* The picture's room: the screen's one picture layer stands over it. */}
              <Layer
                height={pictureWidth / PICTURE_RATIO}
                inFlow
                pointerEvents="none"
                testID="watch-window-room"
                width="100%"
              />
              <Surface
                alignItems="center"
                backgroundColor="transparent"
                borderWidth={0}
                flexDirection="row"
                gap={12}
                justifyContent="space-between"
                position="relative"
                ref={partRef(windowLine)}
                testID="watch-window-line"
                {...web({ onMouseDown: pressChrome })}
              >
                <Layer left={0} pointerEvents="none" right={0} top={0}>
                  {bars}
                </Layer>
                {watched ? (
                  <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
                    {third(watched, true)}
                  </Surface>
                ) : null}
                {quietRowOf(false)}
              </Surface>
              {header}
              {hint}
              {listOf({
                renderPinned: (channel) => (
                  <Surface
                    alignItems="center"
                    backgroundColor="transparent"
                    borderWidth={0}
                    flexDirection="row"
                    gap={12}
                    minHeight={44}
                  >
                    <Text textRole="mono" tone="secondary">
                      {channel.number}{" "}
                    </Text>
                    <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
                      <Text numberOfLines={1} textAlign="left" textRole="cardLabel">
                        {channel.name}{" "}
                      </Text>
                      {channel.onNow ? (
                        <Text numberOfLines={1} textAlign="left" textRole="compact" tone="secondary">
                          {channel.onNow}{" "}
                        </Text>
                      ) : null}
                    </Surface>
                    {/* The list says "watching" to a screen reader itself. */}
                    <Text aria-hidden textRole="metadata" tone="secondary">
                      watching
                    </Text>
                  </Surface>
                ),
                scrollMemory: narrowScroll,
              })}
            </Surface>
            {menu ? (
              <Layer bottom={MENU_FOOT} right={NARROW_EDGE} testID="watch-menu" zIndex={2}>
                {menu}
              </Layer>
            ) : null}
          </ThemeScope>
        </>
      );
    } else {
      // The picture is the largest 16:9 box its room holds, standing on the line under it. Until the
      // room is measured it is guessed from the screen.
      const roomOf: Rect = room ?? {
        height: Math.max(0, box.height - WINDOW_TOP - WINDOW_FOOT - GUESSED_UNDER),
        width: Math.max(0, box.width - WINDOW_EDGE * 2 - WINDOW_LIST_WIDTH - WINDOW_GAP),
        x: 0,
        y: 0,
      };
      const pictureWidth = Math.min(roomOf.width, roomOf.height * PICTURE_RATIO);
      const pictureHeight = pictureWidth / PICTURE_RATIO;
      pictureAt = {
        height: pictureHeight,
        left: WINDOW_EDGE + WINDOW_LIST_WIDTH + WINDOW_GAP + roomOf.x,
        top: WINDOW_TOP + roomOf.y + roomOf.height - pictureHeight,
        width: pictureWidth,
      };
      const looked = view.looking;
      const label =
        looked && watched
          ? `Looking at ${looked.number} ${looked.name} · still playing ${watched.number} ${watched.name} · click to tune`
          : watched
            ? `Today on ${watched.name}`
            : "";

      // The strip and the facts are the shown channel's: the one looked at, else the one watched,
      // which plays behind live by the time paused.
      const dayOf = view.shown;
      const airings = dayOf && schedule ? schedule.airings(dayOf) : [];
      const nowMs = schedule?.nowMs ?? 0;
      const headMs = dayOf?.id === state.watchedId ? nowMs - view.behindSeconds * 1000 : nowMs;
      const onAir = airings.find((airing) => airing.startMs <= headMs && headMs < airing.stopMs);
      const stripFrom = onAir?.startMs ?? headMs;
      const pointedLook = pointed && pointed.channelId === dayOf?.id ? pointed.look : undefined;
      const factsOf =
        pointedLook ?? (onAir && { airing: onAir, startMs: onAir.startMs, stopMs: onAir.stopMs });

      chrome = (
        <>
          <ThemeScope theme="dark">
            <Surface
              backgroundColor="surfaceCanvas"
              borderRadius={0}
              borderWidth={0}
              flex={1}
              flexDirection="row"
              gap={WINDOW_GAP}
              paddingBottom={WINDOW_FOOT}
              paddingHorizontal={WINDOW_EDGE}
              paddingTop={WINDOW_TOP}
              ref={partRef(windowRoot)}
              testID="watch-window"
            >
              <Surface
                backgroundColor="transparent"
                borderWidth={0}
                gap={6}
                minHeight={0}
                width={WINDOW_LIST_WIDTH}
              >
                {header}
                {listOf({
                  onLook: (channelId) => send({ channelId, type: "look" }),
                  // The tune's bars run along the crossing, the top of the pinned lower third.
                  renderPinned: (channel) => (
                    <Surface
                      backgroundColor="transparent"
                      borderWidth={0}
                      paddingVertical={12}
                      position="relative"
                    >
                      <Layer left={0} pointerEvents="none" right={0} top={0}>
                        {bars}
                      </Layer>
                      {third(channel, false)}
                    </Surface>
                  ),
                  ...(lineTop ? { restFromListTop: lineTop + lineHeight - listTop } : {}),
                  scrollMemory: wideScroll,
                })}
                {hint}
              </Surface>

              <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
                {/* The picture's room: the screen's one picture layer stands over it. */}
                <Layer
                  flex={1}
                  inFlow
                  onLayout={(event: LayoutChangeEvent) => setRoom(rectOf(event))}
                  pointerEvents="none"
                  testID="watch-window-room"
                />
                <Surface
                  backgroundColor="transparent"
                  borderWidth={0}
                  onLayout={(event: LayoutChangeEvent) => setLineTop(event.nativeEvent.layout.y)}
                  ref={partRef(windowLine)}
                  testID="watch-window-line"
                  {...web({ onMouseDown: pressChrome })}
                >
                  <Surface
                    alignItems="center"
                    backgroundColor="transparent"
                    borderWidth={0}
                    flexDirection="row"
                    gap={16}
                    justifyContent="space-between"
                    minHeight={LINE_HEIGHT}
                    onLayout={(event: LayoutChangeEvent) => setLineHeight(event.nativeEvent.layout.height)}
                  >
                    <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
                      <Text numberOfLines={1} testID="watch-window-label" textRole="compact" tone="secondary">
                        {label}
                      </Text>
                    </Surface>
                    {quietRowOf(view.behindSeconds > 0)}
                  </Surface>
                  {dayOf && schedule ? (
                    <FilmStrip
                      airings={airings}
                      formatTime={schedule.formatTime}
                      key={dayOf.id}
                      nowMs={nowMs}
                      onLook={(look) => setPointed(look ? { channelId: dayOf.id, look } : undefined)}
                      playheadMs={headMs}
                      ref={strip}
                      window={{ startMs: stripFrom, stopMs: stripFrom + STRIP_SPAN_MS }}
                    />
                  ) : null}
                  <Surface
                    backgroundColor="transparent"
                    borderWidth={0}
                    marginTop={FACTS_GAP}
                    minHeight={FACTS_HEIGHT}
                    testID="watch-window-facts"
                  >
                    {factsOf && schedule ? (
                      <ProgrammeFacts
                        airing={factsOf.airing}
                        formatTime={schedule.formatTime}
                        nowMs={nowMs}
                        startMs={factsOf.startMs}
                        stopMs={factsOf.stopMs}
                      />
                    ) : null}
                  </Surface>
                </Surface>
              </Surface>
            </Surface>
            {menu ? (
              <Layer bottom={menuFoot} right={WINDOW_EDGE} testID="watch-menu" zIndex={2}>
                {menu}
              </Layer>
            ) : null}
          </ThemeScope>
        </>
      );
    }
  }

  return (
    <Layer
      fill
      onLayout={(event: LayoutChangeEvent) => setMeasured(sizeOf(event))}
      testID="watch-screen"
      {...web({ onPointerMove })}
    >
      {/* The one picture layer, the same element in every arrangement, so a change of arrangement
          never remounts the player. Each arrangement says where it stands: at full screen it fills
          the screen under the chrome; in a window it stands over the window's canvas. */}
      <Layer testID="watch-picture-layer" {...pictureAt} {...(inWindow ? { zIndex: 1 } : {})}>
        {pictureStack}
      </Layer>
      {chrome}
      {liveRegion}
    </Layer>
  );
};

export { WatchScreen };
