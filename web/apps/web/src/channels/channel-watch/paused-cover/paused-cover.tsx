import { Stack, ThemeScope } from "@loomarr/design-system";
import { PlayerCover } from "@loomarr/ui";
import type { ComponentProps } from "react";

// A paused channel has nothing to watch: the off-air cover takes the whole frame.
const PausedCover = ({
  channelLabel,
  channelName,
  density,
  isAdmin,
  onLeave,
}: {
  channelLabel: string;
  channelName: string;
  density: ComponentProps<typeof PlayerCover>["density"];
  isAdmin: boolean;
  onLeave: () => void;
}) => (
  <Stack className="relative min-h-0" flexBasis="0%" flexGrow={1} render="section">
    <ThemeScope theme="dark">
      <PlayerCover
        channelLabel={channelLabel}
        density={density}
        kind="off-air"
        // The web mock's words; the way back is an admin's, so members get the first sentence.
        message={
          isAdmin
            ? "This channel is paused, so there's nothing to watch. Resume it under Danger zone."
            : "This channel is paused, so there's nothing to watch."
        }
        onLeave={onLeave}
        title={`${channelName} is off air`}
      />
    </ThemeScope>
  </Stack>
);

export { PausedCover };
