import { ThemeScope } from "@loomarr/design-system";
import type { PlayerFailureCause } from "@loomarr/player";
import { PlayerCover } from "@loomarr/ui";
import type { ComponentProps } from "react";

// What the stopped cover says for each cause, in plain words (the H mock's copy). A channel the app
// can't play has no cause and reads as a stream that stopped.
const STOPPED_COVER: Record<PlayerFailureCause | "none", { message?: string; title: string }> = {
  channel: {
    message:
      "Loomarr is running, but it couldn't start the video for this channel. Trying again usually fixes it.",
    title: "This channel couldn't start",
  },
  none: { title: "The stream stopped." },
  server: {
    message:
      "The picture stopped because this device lost touch with Loomarr. Check that the server is running and that you're on the same network, then try again.",
    title: "Loomarr's server isn't answering",
  },
  stream: { message: "Trying again usually brings it back.", title: "The stream stopped." },
};

// A stream that stopped has nothing for the playbar to stand on, so Retry and the way out take the
// frame in the chrome's place.
const StoppedCover = ({
  channelLabel,
  density,
  failureCause,
  onLeave,
  onRetry,
}: {
  channelLabel: string;
  density: ComponentProps<typeof PlayerCover>["density"];
  failureCause: PlayerFailureCause | undefined;
  onLeave: () => void;
  onRetry: () => void;
}) => (
  <ThemeScope theme="dark">
    <PlayerCover
      channelLabel={channelLabel}
      density={density}
      kind="error"
      onLeave={onLeave}
      message={STOPPED_COVER[failureCause ?? "none"].message}
      onRetry={onRetry}
      title={STOPPED_COVER[failureCause ?? "none"].title}
    />
  </ThemeScope>
);

export { StoppedCover };
