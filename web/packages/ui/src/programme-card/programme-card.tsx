import {
  ArtworkFrame,
  type Density,
  FocusSurface,
  ProgressTrack,
  Surface,
  semanticColors,
  Text,
} from "@loomarr/design-system";
import { Platform } from "react-native";

import { ChannelIdentity, ProgrammeIdentity } from "../identity";

import type { ProgrammeCardData, ProgrammeCardProps, ProgrammeCardViewer } from "./programme-card.type";

// ONE card model for "what is on" (#1659, map: Home's Watching now, the Guide's On now, native's
// programme card). The layouts differ in where the words sit, not in what they are: every layout
// reads the same `ProgrammeCardData`, the same artwork states and the same progress.
//
// Missing artwork is the frame's own state ("No artwork" on the placeholder surface) in every
// layout, so the maintainer's artwork fallback replaces it in one place.

const programmeLine = (programme: ProgrammeCardData) =>
  programme.seriesTitle ? `${programme.seriesTitle} · ${programme.title}` : programme.title;

// The channel line both web layouts share: the amber mono number, then the name.
const ChannelLine = ({ density, programme }: { density: Density; programme: ProgrammeCardData }) => (
  <Surface
    alignItems="center"
    backgroundColor="$transparent"
    borderWidth={0}
    flexDirection="row"
    gap="$inline"
  >
    <Text density={density} textRole="cardChannelNumber">
      {programme.channelNumber}
    </Text>
    <Text density={density} flexShrink={1} numberOfLines={1} textRole="cardMeta">
      {programme.channelName}
    </Text>
  </Surface>
);

// On now (#1659 web mock): 16:9 still with a 3 px progress line along its foot, then the channel,
// the programme and the time left.
const StackedCard = ({ artwork, density, programme }: ProgrammeCardProps & { density: Density }) => (
  <Surface
    backgroundColor="$surfaceRaised"
    borderColor="$borderDecorative"
    borderRadius="$cardCompact"
    overflow="hidden"
    width="100%"
  >
    <Surface backgroundColor="$transparent" borderWidth={0} position="relative" width="100%">
      <ArtworkFrame
        borderRadius={0}
        borderWidth={0}
        density={density}
        state={programme.artworkState}
        width="100%"
      >
        {artwork}
      </ArtworkFrame>
      {programme.progressPercent === undefined ? null : (
        <ProgressTrack
          bottom={0}
          ends="square"
          height={3}
          left={0}
          percent={programme.progressPercent}
          position="absolute"
          right={0}
          tone="artwork"
        />
      )}
    </Surface>
    <Surface
      backgroundColor="$transparent"
      borderWidth={0}
      gap={3}
      paddingBottom={12}
      paddingHorizontal={12}
      paddingTop={10}
    >
      <ChannelLine density={density} programme={programme} />
      <Text density={density} numberOfLines={1} textRole="cardTitle">
        {programmeLine(programme)}
      </Text>
      <Text density={density} textRole="cardTime">
        {programme.timeLabel}
      </Text>
    </Surface>
  </Surface>
);

// The overlay's gradient. CSS on the web; native has no gradient primitive yet, so it falls back to
// the existing solid scrim, which keeps the text legible over any still.
const scrimGradient = `linear-gradient(180deg, rgba(11, 12, 14, 0) 35%, ${semanticColors.artwork.scrimStrong})`;

const ViewerLine = ({ density, viewer }: { density: Density; viewer: ProgrammeCardViewer }) => (
  <Surface
    alignItems="center"
    backgroundColor="$transparent"
    borderWidth={0}
    flexDirection="row"
    gap="$inline"
  >
    <Surface
      alignItems="center"
      backgroundColor="$borderDecorative"
      borderRadius="$round"
      borderWidth={0}
      height={22}
      justifyContent="center"
      width={22}
    >
      <Text density={density} textRole="cardInitials">
        {viewer.initials}
      </Text>
    </Surface>
    <Text density={density} textRole="cardLabel">
      {viewer.name}
    </Text>
    {viewer.device ? (
      <Text density={density} numberOfLines={1} textRole="caption" tone="secondary">
        · {viewer.device}
      </Text>
    ) : null}
  </Surface>
);

// Watching now (#1659 web mock): the whole card is the still, and the viewer, programme, channel
// and progress sit on its gradient.
const OverlayCard = ({
  artwork,
  density,
  highlighted = false,
  hoverHighlight = false,
  programme,
  viewer,
}: ProgrammeCardProps & { density: Density }) => (
  <Surface
    aspectRatio={16 / 9}
    backgroundColor="$surfaceElevated"
    borderColor={highlighted ? "$actionFocus" : "$borderDecorative"}
    hoverStyle={hoverHighlight ? { borderColor: "$actionFocus" } : undefined}
    borderRadius="$cardCompact"
    overflow="hidden"
    position="relative"
    width="100%"
  >
    <ArtworkFrame
      borderRadius={0}
      borderWidth={0}
      bottom={0}
      density={density}
      left={0}
      position="absolute"
      right={0}
      state={programme.artworkState}
      top={0}
    >
      {artwork}
    </ArtworkFrame>
    <Surface
      borderWidth={0}
      bottom={0}
      left={0}
      pointerEvents="none"
      position="absolute"
      right={0}
      top={0}
      {...(Platform.OS === "web"
        ? { backgroundColor: "$transparent", style: { backgroundImage: scrimGradient } }
        : { backgroundColor: "$artworkScrim" })}
    />
    <Surface
      backgroundColor="$transparent"
      borderWidth={0}
      bottom={12}
      gap={4}
      left={14}
      pointerEvents="none"
      position="absolute"
      right={14}
    >
      {viewer ? <ViewerLine density={density} viewer={viewer} /> : null}
      <Text density={density} numberOfLines={1} textRole="cardTitle">
        {programmeLine(programme)}
      </Text>
      <ChannelLine density={density} programme={programme} />
      {programme.progressPercent === undefined ? null : (
        <ProgressTrack height={3} percent={programme.progressPercent} tone="artwork" width="100%" />
      )}
    </Surface>
  </Surface>
);

const ProgrammeCard = (props: ProgrammeCardProps) => {
  const { artwork, channelLogo, density = "pointer", focused = false, layout = "default", programme } = props;
  if (layout === "stacked") return <StackedCard {...props} density={density} />;
  if (layout === "overlay") return <OverlayCard {...props} density={density} />;

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
        <ProgressTrack percent={programme.progressPercent} width="100%" />
      )}
    </FocusSurface>
  );
};

export { ProgrammeCard };
