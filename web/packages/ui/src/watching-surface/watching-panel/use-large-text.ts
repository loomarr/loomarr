import { useWindowDimensions } from "react-native";

// iOS scales text by 1.0 at the default size and about 1.24 at the largest ordinary size
// (xxxLarge); the accessibility sizes begin at AX1, about 1.65. Halfway between is where a row of four
// controls stops fitting.
const LARGE_TEXT_FONT_SCALE = 1.5;

// The viewer's text size is at an accessibility size (AX1 and up). The window's font scale follows a
// change made in Settings while the app is open, which `PixelRatio.getFontScale()` would not.
const isLargeText = (fontScale: number) => fontScale >= LARGE_TEXT_FONT_SCALE;

const useLargeText = () => isLargeText(useWindowDimensions().fontScale);

export { isLargeText, LARGE_TEXT_FONT_SCALE, useLargeText };
