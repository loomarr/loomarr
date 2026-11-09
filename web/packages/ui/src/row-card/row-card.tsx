import { Surface, Text } from "@loomarr/design-system";

import { CardMeta } from "../card-parts";
import type { RowCardProps } from "./row-card.type";

// RowCard — the family for things without a picture (#2765, Direction mock "Two families"):
// channels, sources, people, settings. A small round lead (an icon or an initial), the title, ONE
// metadata line and ONE action. Lives inside a `ListGroup`, which draws the hairlines.
const RowCard = ({ action, lead, meta, title }: RowCardProps) => (
  <Surface
    alignItems="center"
    backgroundColor="transparent"
    borderWidth={0}
    flexDirection="row"
    gap={12}
    minWidth={0}
    paddingHorizontal={12}
    paddingVertical={10}
  >
    <Surface
      alignItems="center"
      backgroundColor="surfaceElevated"
      borderRadius="round"
      borderWidth={0}
      flexShrink={0}
      height={36}
      justifyContent="center"
      width={36}
    >
      {lead}
    </Surface>
    <Surface backgroundColor="transparent" borderWidth={0} flexBasis={0} flexGrow={1} gap={2} minWidth={0}>
      <Text as="p" textRole="cardTitle">
        {title}
      </Text>
      <CardMeta parts={meta} />
    </Surface>
    {action}
  </Surface>
);

export { RowCard };
