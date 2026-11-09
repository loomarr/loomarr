import { Surface, Text } from "@loomarr/design-system";

import { CardMeta } from "../card-parts";
import type { PictureCardProps, PictureRatio } from "./picture-card.type";

// The fixed media width per ratio in a `left` card, so every row of one kind lines up.
const LEFT_WIDTH: Record<PictureRatio, number> = { "1/1": 64, "16/9": 112, "2/3": 64 };
const ASPECT: Record<PictureRatio, number> = { "1/1": 1, "16/9": 16 / 9, "2/3": 2 / 3 };

// PictureCard — the family for things with a picture (#2765, Direction mock "Two families"):
// programmes, titles, clips, requests. The picture at a fixed ratio, then the ident chip, the
// title, ONE metadata line and ONE action at the trailing edge.
const PictureCard = ({ action, chip, corner, media, meta, orientation, ratio, title }: PictureCardProps) => {
  const picture = (
    <Surface
      aspectRatio={ASPECT[ratio]}
      backgroundColor="surfaceElevated"
      borderRadius={0}
      borderWidth={0}
      overflow="hidden"
      position="relative"
      width="100%"
    >
      {media}
      {corner ? (
        <Surface
          backgroundColor="artworkScrimStrong"
          borderRadius={4}
          borderWidth={0}
          bottom={6}
          paddingHorizontal={6}
          paddingVertical={2}
          pointerEvents="none"
          position="absolute"
          right={6}
        >
          <Text textRole="cardTime">{corner}</Text>
        </Surface>
      ) : null}
    </Surface>
  );
  const words = (
    <>
      <Surface
        alignItems="flex-start"
        backgroundColor="transparent"
        borderWidth={0}
        flexBasis={0}
        flexGrow={1}
        flexShrink={1}
        gap={4}
        minWidth={128}
      >
        {chip}
        <Text as="h3" textRole="cardTitle">
          {title}
        </Text>
        <CardMeta parts={meta} />
      </Surface>
      {action}
    </>
  );
  if (orientation === "top") {
    return (
      <Surface borderRadius="cardCompact" minWidth={0} overflow="hidden" role="article">
        {picture}
        <Surface
          alignItems="center"
          backgroundColor="transparent"
          borderWidth={0}
          columnGap={12}
          flexDirection="row"
          flexWrap="wrap"
          padding={12}
          rowGap={8}
        >
          {words}
        </Surface>
      </Surface>
    );
  }
  return (
    <Surface
      alignItems="center"
      backgroundColor="transparent"
      borderWidth={0}
      columnGap={12}
      flexDirection="row"
      minWidth={0}
      padding={12}
      role="article"
    >
      <Surface
        backgroundColor="transparent"
        borderRadius={6}
        borderWidth={0}
        flexShrink={0}
        overflow="hidden"
        width={LEFT_WIDTH[ratio]}
      >
        {picture}
      </Surface>
      {words}
    </Surface>
  );
};

export { PictureCard };
