import { ArtworkFrame, FocusSurface, ProgressTrack } from "@loomarr/design-system";

import { ChannelIdentity, ProgrammeIdentity } from "../identity";

import type { ProgrammeCardProps } from "./programme-card.type";

const ProgrammeCard = ({
  artwork,
  channelLogo,
  density = "pointer",
  focused = false,
  programme,
}: ProgrammeCardProps) => {
  const padding = density === "tv" ? 24 : 16;
  const maxWidth = density === "tv" ? 760 : density === "touch" ? 620 : 560;

  return (
    <FocusSurface focused={focused} gap="$control" maxWidth={maxWidth} padding={padding} width="100%">
      <ArtworkFrame density={density} state={programme.artworkState} width="100%">
        {artwork}
      </ArtworkFrame>

      <ProgrammeIdentity density={density} programme={programme} />
      <ChannelIdentity channel={programme} density={density} logo={channelLogo} />

      {programme.progressPercent === undefined ? null : (
        <ProgressTrack
          accessibilityLabel={programme.title}
          percent={programme.progressPercent}
          width="100%"
        />
      )}
    </FocusSurface>
  );
};

export { ProgrammeCard };
