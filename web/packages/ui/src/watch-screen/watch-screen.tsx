import type { LineupChannel, LineupGroupKind } from "@loomarr/core/channel-lineup";
import {
  Layer,
  Surface,
  Text,
  ThemeScope,
  useReducedMotionPreference,
  VisuallyHidden,
} from "@loomarr/design-system";
import { useEffect, useRef } from "react";
import { Animated, Platform } from "react-native";

import { ChannelList, type ChannelListHandle } from "../channel-list";
import { FirstRunHint } from "../first-run-hint";
import { LowerThird, type LowerThirdState } from "../lower-third";
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

  // Focus goes where the model hands it: onto the list's cursor, or back to Channels.
  // The model refuses a pause while there is no picture to hold.
  const showing = cover ? "covered" : tune.cover === "cold" ? "tuning" : "playing";
  useEffect(() => {
    send({ picture: showing, type: "picture" });
  }, [send, showing]);

  const handOff = state.handOff;
  // biome-ignore lint/correctness/useExhaustiveDependencies: each hand-off moves focus once
  useEffect(() => {
    if (handOff?.to === "list") list.current?.focusCursor();
    if (handOff?.to === "channels") quietRow.current?.focusHead();
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
      onGoLive={() => send({ control: "live", type: "press" })}
      onGoLiveDropped={() => quietRow.current?.focusHead()}
      onGoLiveFocus={(focused) => send({ control: focused ? "live" : undefined, type: "focus" })}
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

  // The pointer resting on the controls holds the screen awake.
  const holdOn = () =>
    web({
      onPointerEnter: () => send({ held: true, type: "pointerHold" }),
      onPointerLeave: () => send({ held: false, type: "pointerHold" }),
    });
  const onPointerMove = () => {
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
        {view.hint ? (
          <Layer centred opacity={shown} top={HINT_TOP}>
            <FirstRunHint device="full" />
          </Layer>
        ) : null}

        {watched && !view.listOpen ? (
          <Layer bottom={CROSSING} left={EDGE} opacity={shown} testID="watch-lower-third" {...holdOn()}>
            {third(watched, true)}
          </Layer>
        ) : null}

        {view.listOpen ? (
          <Layer bottom={LIST_FOOT} left={EDGE} testID="watch-list" top={LIST_TOP} width={LIST_WIDTH}>
            <ChannelList
              autoFocus
              detail={detail}
              lineup={lineup}
              onEvent={(event) => send({ event, type: "list" })}
              ref={list}
              renderPinned={(channel) => third(channel, false)}
              restFromBottom={CROSSING - LIST_FOOT}
              state={state.list}
            />
          </Layer>
        ) : null}

        <Layer bottom={ROW_FOOT} opacity={shown} right={EDGE} testID="watch-quiet-row" {...holdOn()}>
          <QuietRow
            device="full"
            group={GROUP_NAMES[state.list.group]}
            listOpen={view.listOpen}
            muted={muted ?? false}
            onFocusKey={(key) => send({ control: key, type: "focus" })}
            onPress={(key) => send({ control: key, type: "press" })}
            ownInk={false}
            paused={view.paused}
            ref={quietRow}
            {...(openKey ? { openKey } : {})}
          />
        </Layer>

        {view.menu && renderMenu ? (
          <Layer bottom={MENU_FOOT} right={EDGE} testID="watch-menu">
            {renderMenu({
              close: () => send({ type: "closeMenu" }),
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
    </Layer>
  );
};

export { WatchScreen };
