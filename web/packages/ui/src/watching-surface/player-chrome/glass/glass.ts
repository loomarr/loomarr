import { playerChrome } from "@loomarr/design-system";
import { Platform, type ViewStyle } from "react-native";

// Player-chrome elevation (frontend-design §9.1): over moving video a hairline separates nothing,
// so each plane is glass — a translucent fill lit from the top, a faint ring, backdrop blur and a
// soft drop shadow. React Native has no backdrop blur and no CSS gradients, so a native client gets
// the same luminance as a solid tinted fill; the plane order still reads.

const web = Platform.OS === "web";

/** A web-only style (gradients, blur, layered shadows), or nothing on native. */
const webOnly = (style: Record<string, string | number>): ViewStyle => (web ? (style as ViewStyle) : {});

/** A CSS gradient on the web; the solid colour it averages to on native. */
const gradientOr = (gradient: string, solid: string): ViewStyle =>
  web ? ({ backgroundImage: gradient } as ViewStyle) : { backgroundColor: solid };

/** The lit glass of a capsule or the playbar's tray. */
const glass = (shadow: string = playerChrome.shadowCapsule): ViewStyle => ({
  backgroundColor: web ? "transparent" : playerChrome.glassSolid,
  ...webOnly({
    backdropFilter: `blur(${playerChrome.glassBlur}px)`,
    backgroundImage: `linear-gradient(180deg, ${playerChrome.glassTop}, ${playerChrome.glassBottom})`,
    boxShadow: `inset 0 1px 0 ${playerChrome.glassHighlight}, 0 0 0 1px ${playerChrome.glassRing}, ${shadow}`,
  }),
});

/** The darker glass of the band's panel and the preview card: text sits on it, so it is denser. */
const panelGlass = (shadow: string = playerChrome.shadowPlane): ViewStyle => ({
  backgroundColor: web ? "transparent" : playerChrome.glassSolid,
  ...webOnly({
    backdropFilter: `blur(${playerChrome.glassBlur}px)`,
    backgroundImage: `linear-gradient(180deg, ${playerChrome.panelTop}, ${playerChrome.panelBottom})`,
    boxShadow: `inset 0 1px 0 ${playerChrome.glassHighlight}, 0 0 0 1px ${playerChrome.glassRing}, ${shadow}`,
  }),
});

export { glass, gradientOr, panelGlass, webOnly };
