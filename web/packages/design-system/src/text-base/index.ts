import { styled, Text } from "@tamagui/core";

/**
 * The Tamagui text every design-system text draws with. Tamagui 3 only resolves the text variants
 * (`numberOfLines`' ellipsis and line clamp, `ellipsis`) for a `styled()` component; the bare core
 * `Text` silently drops them on web, which would let one-line labels overflow instead of clipping.
 */
const TamaguiText = styled(Text, {});

export { TamaguiText };
