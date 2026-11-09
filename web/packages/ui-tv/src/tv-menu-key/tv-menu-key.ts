import { useEffect } from "react";
import { Platform } from "react-native";

/** The part of react-native-tvos's `TVEventControl` the hook drives. */
interface TvMenuKeyControl {
  disableTVMenuKey: () => void;
  enableTVMenuKey: () => void;
}

interface TvPlatform {
  OS: string;
  isTV: boolean;
}

/** tvOS is iOS on a TV; react-native-tvos 0.88's types no longer declare `Platform.isTVOS`. */
const isTvOS = (platform: TvPlatform = Platform): boolean => platform.OS === "ios" && platform.isTV;

/**
 * Whether the viewer's remote has number keys. The Siri Remote has none (#1500 audit #3), so Apple TV
 * has no digit entry: no "0–9 jump" hint and no channel-number wait to tune.
 */
const tvRemoteHasDigits = (platform: TvPlatform = Platform): boolean => !isTvOS(platform);

/**
 * tvOS gives the Menu key to the app only while it is enabled, and then nothing else acts on it, so
 * enable it only while a screen has somewhere to go back to. Everywhere else Menu goes to the Home
 * screen, as Apple's HIG expects at an app's root. A modal dialog takes Menu itself. Android TV's
 * Back always reaches BackHandler, so it is left alone.
 *
 * The TV app passes react-native-tvos's `TVEventControl`: react-native-web, which bundles this
 * package for the web app, has no such export.
 */
const useTvMenuKey = (
  enabled: boolean,
  { control, tvOS = isTvOS() }: { control: TvMenuKeyControl; tvOS?: boolean },
) => {
  useEffect(() => {
    if (!enabled || !tvOS) return undefined;
    control.enableTVMenuKey();
    return () => control.disableTVMenuKey();
  }, [control, enabled, tvOS]);
};

export type { TvMenuKeyControl, TvPlatform };
export { isTvOS, tvRemoteHasDigits, useTvMenuKey };
