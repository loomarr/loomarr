import {
  Icon,
  type IconName,
  icons,
  RingedPressable,
  type RingedPressableHandle,
  Surface,
  Text,
  ThemeScope,
} from "@loomarr/design-system";
import { Fragment, type Ref, useImperativeHandle, useLayoutEffect, useRef, useState } from "react";
import { Platform } from "react-native";

import { focusHost } from "../focus-host";
import { PictureInk } from "../picture-ink";

type QuietRowDevice = "landscape" | "portrait" | "tv" | "full" | "window";

type QuietRowKey =
  | "airplay"
  | "audio"
  | "channels"
  | "fullscreen"
  | "guide"
  | "live"
  | "more"
  | "pause"
  | "subtitles"
  | "volume";

interface QuietRowProps {
  /** `full` is a desktop at full screen, `window` a desktop window. */
  device: QuietRowDevice;
  /**
   * Watching behind live; a television and a desktop window then lead with Go live unless paused,
   * since neither has a lower third whose Go live takes focus.
   */
  behindLive?: boolean;
  /** The channel group in play; anything but `All` is named on the Channels control. */
  group?: string;
  /** The channel list is open, so Channels shows as on. */
  listOpen?: boolean;
  muted?: boolean;
  /** The control whose menu is open. */
  openKey?: QuietRowKey;
  onPress: (key: QuietRowKey) => void;
  /**
   * A control took the keyboard's or the remote's focus, or that focus left the row (`undefined`).
   * Focus a pointer gave is not said.
   */
  onFocusKey?: (key: QuietRowKey | undefined) => void;
  /**
   * Over the picture the row stands on a foot scrim of its own. A screen that draws one scrim under
   * the row and the lower third together passes `false`; the row stays in the dark theme.
   */
  ownInk?: boolean;
  /**
   * A control left from under the focus (a television's Go live, once pressed). Given, the row
   * says so and the screen decides where focus goes; left out, focus goes to the head of the row.
   */
  onFocusDropped?: () => void;
  paused?: boolean;
  ref?: Ref<QuietRowHandle>;
}

interface QuietRowHandle {
  /** Moves focus to the control at the head of the row. */
  focusHead: () => void;
  /** Moves focus to one control of the row, when the row has it. */
  focusControl: (key: QuietRowKey) => void;
}

const KEYS: Record<QuietRowDevice, readonly QuietRowKey[]> = {
  full: ["channels", "guide", "pause", "subtitles", "audio", "volume", "fullscreen", "more"],
  landscape: ["channels", "guide", "pause", "subtitles", "audio", "airplay", "more"],
  portrait: ["subtitles", "audio", "airplay"],
  tv: ["channels", "guide", "pause", "subtitles", "audio", "more"],
  window: ["pause", "subtitles", "audio", "volume", "fullscreen", "more"],
};

// A desktop's keys; a television and a phone have none to name.
const SHORTCUTS: Partial<Record<QuietRowKey, string>> = {
  audio: "A",
  fullscreen: "F",
  pause: "Space",
  subtitles: "C",
  volume: "M",
};

type Face = { icon: IconName; name: string };

const faceOf = (
  key: Exclude<QuietRowKey, "channels" | "guide" | "live">,
  { device, muted, paused }: Pick<QuietRowProps, "device" | "muted" | "paused">,
): Face => {
  switch (key) {
    case "pause":
      return paused ? { icon: "play", name: "Play" } : { icon: "pause", name: "Pause" };
    case "volume":
      return muted ? { icon: "volumeMuted", name: "Unmute" } : { icon: "volume", name: "Mute" };
    case "fullscreen":
      return device === "full"
        ? { icon: "fullscreenExit", name: "Leave full screen" }
        : { icon: "fullscreen", name: "Full screen" };
    case "subtitles":
      return { icon: "captions", name: "Subtitles" };
    case "audio":
      return { icon: "audio", name: "Audio" };
    case "airplay":
      return { icon: "airplay", name: "AirPlay" };
    case "more":
      return { icon: "more", name: "More" };
  }
};

const keysFor = ({ behindLive, device, paused }: QuietRowProps): readonly QuietRowKey[] =>
  (device === "tv" || device === "window") && behindLive && !paused
    ? ["live", ...KEYS[device]]
    : KEYS[device];

/**
 * The quiet row of The Cross (frontend-design §9.3, target): one toolbar of controls. It is one Tab
 * stop on the web; Left and Right move between controls, and Home and End go to the ends. It reports
 * presses and opens nothing.
 */
const QuietRow = (props: QuietRowProps) => {
  const {
    device,
    group,
    listOpen = false,
    onFocusDropped,
    onFocusKey,
    onPress,
    openKey,
    ownInk = true,
    ref,
  } = props;
  const controls = useRef(new Map<QuietRowKey, RingedPressableHandle>());
  const focusedKey = useRef<QuietRowKey | undefined>(undefined);
  const keyboardKey = useRef<QuietRowKey | undefined>(undefined);
  const [stopKey, setStopKey] = useState<QuietRowKey | undefined>(undefined);
  const focusKey = (key: QuietRowKey) => focusHost(controls.current.get(key));
  const keys = keysFor(props);
  const stop = stopKey && keys.includes(stopKey) ? stopKey : keys[0];
  const desktop = device === "full" || device === "window";
  const overPicture = device === "full" || device === "landscape" || device === "tv";
  const size = desktop ? 36 : 44;

  const onKeyDown = (event: {
    altKey: boolean;
    ctrlKey: boolean;
    key: string;
    metaKey: boolean;
    preventDefault: () => void;
  }) => {
    if (event.altKey || event.ctrlKey || event.metaKey || !stop) return;
    const here = keys.indexOf(stop);
    const to = { ArrowLeft: here - 1, ArrowRight: here + 1, End: keys.length - 1, Home: 0 }[event.key];
    if (to === undefined) return;
    event.preventDefault();
    const next = keys[Math.min(keys.length - 1, Math.max(0, to))];
    if (next) focusKey(next);
  };

  // Read while rendering, before the control goes: its removal may or may not send a blur. A
  // television's viewer presses Go live and it leaves the row from under the focus.
  const orphaned = useRef(false);
  if (focusedKey.current && !keys.includes(focusedKey.current)) {
    orphaned.current = true;
    focusedKey.current = undefined;
  }
  useLayoutEffect(() => {
    if (!orphaned.current) return;
    orphaned.current = false;
    if (onFocusDropped) onFocusDropped();
    else if (keys[0]) focusKey(keys[0]);
  });
  useImperativeHandle(ref, () => ({
    focusControl: (key: QuietRowKey) => keys.includes(key) && focusKey(key),
    focusHead: () => keys[0] && focusKey(keys[0]),
  }));

  const shared = (key: QuietRowKey) => ({
    onBlur: () => {
      if (focusedKey.current === key) focusedKey.current = undefined;
    },
    onFocus: () => {
      focusedKey.current = key;
      setStopKey(key);
    },
    // Only the keyboard's and the remote's focus is said; a control that loses it says so only if
    // it is still the one said, since the next may take it first.
    onKeyboardFocus: (focused: boolean) => {
      if (focused) {
        keyboardKey.current = key;
        onFocusKey?.(key);
      } else if (keyboardKey.current === key) {
        keyboardKey.current = undefined;
        onFocusKey?.(undefined);
      }
    },
    // A pointer never takes focus here, so a key pressed after a click is the picture's key and
    // draws no ring on a control the viewer never moved to.
    pointerTakesNoFocus: true,
    onPress: () => onPress(key),
    ref: (node: RingedPressableHandle | null) => {
      if (node) controls.current.set(key, node);
      else controls.current.delete(key);
    },
    size,
    // The roving Tab stop is the web's: a native view with a negative tabIndex takes no remote focus.
    tabIndex: (Platform.OS !== "web" || key === stop ? 0 : -1) as 0 | -1,
  });

  const control = (key: QuietRowKey) => {
    if (key === "live") {
      return (
        <RingedPressable accessibilityLabel="Go live" {...shared(key)}>
          <Surface backgroundColor="stateLive" borderRadius={4} borderWidth={0} height={7} width={7} />
          <Text textRole="label" tone="live">
            Go live
          </Text>
        </RingedPressable>
      );
    }
    if (key === "channels" || key === "guide") {
      const grouped = group && group !== "All" ? `Channels: ${group}` : "Channels";
      const words = key === "guide" ? "Guide" : grouped;
      const open = key === "channels" ? listOpen : openKey === "guide";
      return (
        <RingedPressable
          accessibilityLabel={words}
          expanded={key === "channels" ? listOpen : undefined}
          on={open}
          {...shared(key)}
        >
          <Icon
            decorative
            glyph={icons[key === "channels" ? "list" : "guide"]}
            size="control"
            tone="content"
          />
          <Text numberOfLines={1} textRole="label" tone="primary">
            {words}
          </Text>
        </RingedPressable>
      );
    }
    const face = faceOf(key, props);
    const shortcut = desktop ? SHORTCUTS[key] : undefined;
    const tip = shortcut ? `${face.name} (${shortcut})` : face.name;
    return (
      <RingedPressable
        accessibilityLabel={tip}
        expanded={openKey === undefined ? undefined : openKey === key}
        on={openKey === key}
        tip={tip}
        // A television has no hover, so the focused control is named above itself.
        tipOn={device === "tv" ? "focus" : "hover"}
        {...shared(key)}
      >
        <Icon decorative glyph={icons[face.icon]} size="touch" tone="content" />
      </RingedPressable>
    );
  };

  const row = (
    <Surface
      alignItems="center"
      aria-label="Player controls"
      backgroundColor="transparent"
      borderRadius={0}
      borderWidth={0}
      flexDirection="row"
      flexWrap="wrap"
      gap={8}
      justifyContent="flex-end"
      // The DOM takes a view's key events on the web; native moves focus by itself.
      {...(Platform.OS === "web" ? ({ onKeyDown } as object) : {})}
      role="toolbar"
    >
      {keys.map((key) => (
        <Fragment key={key}>
          {control(key)}
          {key === "guide" ? (
            <Surface backgroundColor="borderDecorative" borderWidth={0} height={20} width={1} />
          ) : null}
        </Fragment>
      ))}
    </Surface>
  );

  if (!overPicture) return row;
  return ownInk ? <PictureInk edge="foot">{row}</PictureInk> : <ThemeScope theme="dark">{row}</ThemeScope>;
};

export type { QuietRowDevice, QuietRowHandle, QuietRowKey, QuietRowProps };
export { QuietRow };
