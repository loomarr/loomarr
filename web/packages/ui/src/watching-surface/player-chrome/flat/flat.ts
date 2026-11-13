import { playerChrome } from "@loomarr/design-system";
import { Platform, type ViewStyle } from "react-native";

// Player-chrome surfaces (frontend-design §9.1). The chrome is flat (#2769): a solid fill and one
// hairline, over a picture that has one bottom-up scrim. No gradient, inset highlight, drop shadow or
// backdrop blur, so a native client draws exactly what the web does.

const web = Platform.OS === "web";

/** A web-only style (a transition, a CSS gradient), or nothing on native. */
const webOnly = (style: Record<string, string | number>): ViewStyle => (web ? (style as ViewStyle) : {});

/** The scrim's CSS gradient on the web; the solid colour it averages to on native. */
const gradientOr = (gradient: string, solid: string): ViewStyle =>
  web ? ({ backgroundImage: gradient } as ViewStyle) : { backgroundColor: solid };

/** A solid raised surface with the one hairline: a key, or the strip a row of blocks stands on. */
const raised = (): ViewStyle => ({
  backgroundColor: playerChrome.surfaceRaised,
  borderColor: playerChrome.hairline,
  borderWidth: 1,
});

/**
 * A remote's focus on native, which has no outline: the amber edge as a border (§9.6: focus rings are
 * amber). The web draws its own outline beside this, so it gets nothing here.
 */
const nativeFocus = (focused: boolean): ViewStyle =>
  focused && !web ? { borderColor: playerChrome.signal, borderWidth: 2 } : {};

export { gradientOr, nativeFocus, raised, webOnly };
