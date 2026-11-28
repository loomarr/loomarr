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
import { useCallback, useEffect, useLayoutEffect, useRef } from "react";
import { Animated, Platform, useWindowDimensions } from "react-native";

import { ChannelList, type ChannelListHandle } from "../channel-list";
import { FirstRunHint } from "../first-run-hint";
import { LowerThird, type LowerThirdState } from "../lower-third";
import { summaryOf } from "../lower-third/summary";
import { QuietRow, type QuietRowHandle, type QuietRowKey } from "../quiet-row";
import { TuneBars, TuneStatic } from "../tune-static";
import { clearOfWatermark } from "../watching-surface";
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
/** The back control stands in the top leading corner, on the list's line. */
const BACK_TOP = LIST_TOP;
/** How tall the back control and the quiet row stand, and the lower third, for the logo's clearance. */
const CONTROL_HEIGHT = 36;
const THIRD_HEIGHT = 120;
/** The menu stands over the quiet row. */
const MENU_FOOT = 96;
/** How long the controls take to rest or wake. */
const FADE_MS = 200;
/** Pointer movement wakes the screen; while awake it is reported at most this often. */
const MOVE_EVERY_MS = 500;

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

/**
 * The watching screen of The Cross (frontend-design §9.1, target): the picture with the tune, the
 * scrims, the lower third, the quiet row, the first-run hint and the channel list drawn over it.
 * It draws what `useWatchScreen` says and reports what happened; the caller does what it asks.
 */
const WatchScreen = <C extends LineupChannel>({
  back,
  fullScreen,
  watermark,
  cover,
  detail,
  holdSnow,
  model,
  muted,
  picture,
  programme,
  reducedMotion,
  renderMenu,
  tune,
}: WatchScreenProps<C>) => {
  const { lineup, send, state, view } = model;
  const quietRow = useRef<QuietRowHandle>(null);
  const list = useRef<ChannelListHandle>(null);
  const lastMove = useRef(Number.NEGATIVE_INFINITY);
  const shown = useShown(view.awake, reducedMotion);
  const watched = lineup.channels.find((channel) => channel.id === state.watchedId);

  // The channel's logo is burned into a corner of the picture; only the chrome in that corner steps
  // clear of it, measured on the 16:9 picture inside the screen (§9.6).
  const frame = useWindowDimensions();
  const clear = (base: number, edge: "bottom" | "top", side: "left" | "right", extent: number) =>
    clearOfWatermark({ base, edge, extent, frame, side, watermark });
  const backTop = clear(BACK_TOP, "top", "left", CONTROL_HEIGHT);
  const thirdBottom = clear(CROSSING, "bottom", "left", THIRD_HEIGHT);
  const rowFoot = clear(ROW_FOOT, "bottom", "right", CONTROL_HEIGHT);

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

  // The one pointer rule for the screen's chrome, the lower third, the quiet row and the back
  // control: a press there takes focus off whichever of their controls holds it, so no keyboard
  // ring outlives a click. The open list, the menu and the cover are not part of it.
  type Part = { contains?: (node: unknown) => boolean } | null;
  const footThird = useRef<Part>(null);
  const footRow = useRef<Part>(null);
  const backPart = useRef<Part>(null);
  const pressFoot = () => {
    const active = (globalThis as { document?: { activeElement?: unknown } }).document?.activeElement;
    const parts = [footThird.current, footRow.current, backPart.current];
    if (active && parts.some((part) => part?.contains?.(active))) {
      (active as { blur?: () => void }).blur?.();
    }
  };

  // Focus goes where the model hands it: onto the list's cursor, back to Channels, or back to the
  // control whose menu closed.
  const handOff = state.handOff;
  // biome-ignore lint/correctness/useExhaustiveDependencies: each hand-off moves focus once
  useEffect(() => {
    const to = handOff?.to;
    if (to === "list") handFocus(() => list.current?.focusCursor());
    else if (to === "channels") handFocus(() => quietRow.current?.focusHead());
    else if (to) handFocus(() => quietRow.current?.focusControl(to));
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
      device="full"
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

  // A channel change is said once, with what is on as it changes; the clock moving the lower third
  // on says nothing. The channel the screen opens on is not said.
  const spoken = useRef({ id: state.watchedId, text: "" });
  if (watched && spoken.current.id !== watched.id) {
    spoken.current = { id: watched.id, text: summaryOf(watched.number, watched.name, programme(watched)) };
  }

  // The lower third's element joins the foot, for the pointer rule.
  const thirdArrived = useCallback((node: unknown) => {
    footThird.current = node as Part;
  }, []);
  // The lower third comes back under a pointer that has not moved when the list closes, with no
  // pointer-enter, and a browser has not yet worked out what is under a pointer that is still. So
  // as it is laid out it asks the page what is at the pointer's last place, and holds if that is
  // the lower third. No pointer has moved yet means none rests on it.
  const pointerAt = useRef<{ x: number; y: number } | undefined>(undefined);
  const thirdShown = Boolean(watched) && !view.listOpen;
  useLayoutEffect(() => {
    const at = pointerAt.current;
    const page = (globalThis as { document?: { elementFromPoint?: (x: number, y: number) => unknown } })
      .document;
    if (!thirdShown || !at || Platform.OS !== "web" || !page?.elementFromPoint) return;
    const hit = page.elementFromPoint(at.x, at.y);
    if (hit && footThird.current?.contains?.(hit)) send({ held: true, region: "lowerThird", type: "pointerHold" });
  }, [thirdShown, send]);

  // The pointer resting on the controls holds the screen awake.
  const holdOn = (region: "back" | "lowerThird" | "quietRow") =>
    web({
      onPointerEnter: () => send({ held: true, region, type: "pointerHold" }),
      onPointerLeave: () => send({ held: false, region, type: "pointerHold" }),
    });
  const onPointerMove = (event: { clientX?: number; clientY?: number }) => {
    if (event.clientX !== undefined && event.clientY !== undefined) {
      pointerAt.current = { x: event.clientX, y: event.clientY };
    }
    const at = Date.now();
    if (view.awake && at - lastMove.current < MOVE_EVERY_MS) return;
    lastMove.current = at;
    send({ type: "pointerMove" });
  };
  const openKey =
    view.menu && QUIET_ROW_MENUS.has(view.menu.name) ? (view.menu.name as QuietRowKey) : undefined;

  return (
    <Layer fill testID="watch-screen" {...web({ onPointerMove })}>
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

      <Layer bottom={0} left={0} opacity={shown} pointerEvents="none" right={0} top={0}>
        <WatchScrim edge="foot" />
      </Layer>
      {view.listOpen ? <WatchScrim edge="side" /> : null}

      <ThemeScope theme="dark">
        {/* The arrow is always named beside it. While the list is open its column stands here. */}
        {back && !view.listOpen ? (
          <Layer left={EDGE} opacity={shown} testID="watch-back" top={backTop} {...holdOn("back")}>
            <Surface
              backgroundColor="transparent"
              borderWidth={0}
              ref={(node: unknown) => {
                backPart.current = node as Part;
              }}
              {...web({ onMouseDown: pressFoot })}
            >
              <RingedPressable
                accessibilityLabel={back.label}
                onKeyboardFocus={(focused) => send({ control: focused ? "back" : undefined, type: "focus" })}
                onPress={back.onPress}
                pointerTakesNoFocus
                size={36}
              >
                <Icon decorative glyph={icons.back} size="control" tone="content" />
                <Text numberOfLines={1} textRole="label" tone="primary">
                  {back.label}
                </Text>
              </RingedPressable>
            </Surface>
          </Layer>
        ) : null}

        {view.hint ? (
          <Layer centred opacity={shown} top={HINT_TOP}>
            <FirstRunHint device="full" />
          </Layer>
        ) : null}

        {watched && !view.listOpen ? (
          <Layer
            bottom={thirdBottom}
            left={EDGE}
            opacity={shown}
            testID="watch-lower-third"
            {...holdOn("lowerThird")}
          >
            <Surface
              backgroundColor="transparent"
              borderWidth={0}
              ref={thirdArrived}
              {...web({ onMouseDown: pressFoot })}
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
          bottom={rowFoot}
          opacity={shown}
          right={EDGE}
          testID="watch-quiet-row"
          {...holdOn("quietRow")}
        >
          <Surface
            backgroundColor="transparent"
            borderWidth={0}
            ref={(node: unknown) => {
              footRow.current = node as Part;
            }}
            {...web({ onMouseDown: pressFoot })}
          >
            <QuietRow
              device="full"
              group={GROUP_NAMES[state.list.group]}
              listOpen={view.listOpen}
              muted={muted ?? false}
              {...(fullScreen === undefined ? {} : { fullScreen })}
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

        {view.menu && renderMenu ? (
          <Layer bottom={MENU_FOOT} right={EDGE} testID="watch-menu">
            {renderMenu({
              close: () => send({ fromKey: keyed(), type: "closeMenu" }),
              lastChannel: view.lastChannel,
              name: view.menu.name,
              openPage: (page) => send({ page, type: "menuPage" }),
              page: view.menu.page,
            })}
          </Layer>
        ) : null}
      </ThemeScope>

      {/* The seven bars run along the crossing. */}
      <Layer bottom={CROSSING - 8} left={0} pointerEvents="none" right={0}>
        <TuneBars
          bars={tune.bars}
          run={tune.run}
          {...(reducedMotion === undefined ? {} : { reducedMotion })}
        />
      </Layer>

      <VisuallyHidden>
        <Text accessibilityLiveRegion="polite" textRole="body">
          {announcement}
        </Text>
      </VisuallyHidden>
      <VisuallyHidden>
        <Text accessibilityLiveRegion="polite" testID="watch-channel-said" textRole="body">
          {spoken.current.text}
        </Text>
      </VisuallyHidden>
    </Layer>
  );
};

export { WatchScreen };
