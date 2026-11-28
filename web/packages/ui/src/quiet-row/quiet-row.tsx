import { Icon, type IconName, icons, Surface, Text, Tooltip } from "@loomarr/design-system";
import { Fragment, useRef, useState } from "react";
import { Platform } from "react-native";

import { PictureInk } from "../picture-ink";
import { RingedControl } from "../ringed-control";

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
  /** Watching behind live; a television then leads with Go live unless paused. */
  behindLive?: boolean;
  /** The channel group in play; anything but `All` is named on the Channels control. */
  group?: string;
  /** The channel list is open, so Channels shows as on. */
  listOpen?: boolean;
  muted?: boolean;
  /** The control whose menu is open. */
  openKey?: QuietRowKey;
  onPress: (key: QuietRowKey) => void;
  paused?: boolean;
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
  device === "tv" && behindLive && !paused ? ["live", ...KEYS.tv] : KEYS[device];

/**
 * The quiet row of The Cross (frontend-design §9.3, target): one toolbar of controls. It is one Tab
 * stop on the web; Left and Right move between controls, and Home and End go to the ends. It reports
 * presses and opens nothing.
 */
const QuietRow = (props: QuietRowProps) => {
  const { device, group, listOpen = false, onPress, openKey } = props;
  const controls = useRef(new Map<QuietRowKey, { focus: () => void }>());
  const [stopKey, setStopKey] = useState<QuietRowKey | undefined>(undefined);
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
    if (next) controls.current.get(next)?.focus();
  };

  const shared = (key: QuietRowKey) => ({
    onFocus: () => setStopKey(key),
    onPress: () => onPress(key),
    ref: (node: unknown) => {
      if (node) controls.current.set(key, node as { focus: () => void });
      else controls.current.delete(key);
    },
    size,
    // The roving Tab stop is the web's: a native view with a negative tabIndex takes no remote focus.
    tabIndex: (Platform.OS !== "web" || key === stop ? 0 : -1) as 0 | -1,
  });

  const control = (key: QuietRowKey) => {
    if (key === "live") {
      return (
        <RingedControl accessibilityLabel="Go live" {...shared(key)}>
          <Surface backgroundColor="stateLive" borderRadius={4} borderWidth={0} height={7} width={7} />
          <Text textRole="label" tone="live">
            Go live
          </Text>
        </RingedControl>
      );
    }
    if (key === "channels" || key === "guide") {
      const grouped = group && group !== "All" ? `Channels: ${group}` : "Channels";
      const words = key === "guide" ? "Guide" : grouped;
      const open = key === "channels" ? listOpen : openKey === "guide";
      return (
        <RingedControl
          accessibilityLabel={words}
          aria-expanded={key === "channels" ? listOpen : undefined}
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
        </RingedControl>
      );
    }
    const face = faceOf(key, props);
    const shortcut = desktop ? SHORTCUTS[key] : undefined;
    const tip = shortcut ? `${face.name} (${shortcut})` : face.name;
    const words = <Text textRole="label">{tip}</Text>;
    const button = (
      <RingedControl
        accessibilityLabel={tip}
        aria-expanded={openKey === undefined ? undefined : openKey === key}
        // A native television has no hover and the Tooltip draws nothing, so the row names the
        // focused control itself.
        focusLabel={device === "tv" ? words : undefined}
        on={openKey === key}
        {...shared(key)}
      >
        <Icon decorative glyph={icons[face.icon]} size="touch" tone="content" />
      </RingedControl>
    );
    return device === "tv" ? button : <Tooltip content={tip}>{button}</Tooltip>;
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

  return overPicture ? <PictureInk edge="foot">{row}</PictureInk> : row;
};

export type { QuietRowDevice, QuietRowKey, QuietRowProps };
export { QuietRow };
