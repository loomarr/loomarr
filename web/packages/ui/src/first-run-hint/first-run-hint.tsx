import { Surface, Text, ThemeScope } from "@loomarr/design-system";

import type { QuietRowDevice } from "../quiet-row";

const HINTS: Partial<Record<QuietRowDevice, string>> = {
  full: "Up and Down change channel. Channels shows them all. Guide shows later today",
  portrait: "Tap a channel to change it",
  tv: "Up Down change channel · OK pause · Left Right controls · Back Home",
  window: "Click a channel to change it, or press Up and Down",
};

/**
 * The first-run hint of The Cross (frontend-design §9.2, target): one line per device. The screen
 * decides when it goes. A phone on its side has none.
 */
const FirstRunHint = ({ device }: { device: QuietRowDevice }) => {
  const words = HINTS[device];
  if (!words) return null;
  const plate = (
    <Surface
      alignSelf="center"
      backgroundColor="surfaceCanvas"
      borderRadius={8}
      borderWidth={0}
      paddingHorizontal={12}
      paddingVertical={8}
      role="note"
    >
      <Text textAlign="center" textRole="compact" tone="primary">
        {words}
      </Text>
    </Surface>
  );
  return device === "full" || device === "tv" ? <ThemeScope theme="dark">{plate}</ThemeScope> : plate;
};

export { FirstRunHint };
