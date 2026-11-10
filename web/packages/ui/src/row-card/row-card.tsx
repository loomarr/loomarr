import { Surface, Text } from "@loomarr/design-system";

import { CardMeta } from "../card-parts";
import type { RowCardProps } from "./row-card.type";

const LEAD = 36;
const GAP = 12;
// The narrowest a title is drawn beside the action. A row with less room than this puts the action
// on a line of its own (#3429): on a 390px phone a filler source's name otherwise gets about 121px
// and wraps a word per line.
const TITLE_FLOOR = 180;

// RowCard — the family for things without a picture (#2765, Direction mock "Two families"):
// channels, sources, people, settings. A small round lead (an icon or an initial), the title, ONE
// metadata line and ONE action. Lives inside a `ListGroup`, which draws the hairlines.
//
// The lead and the words are one group, the action the other. The group asks for the lead, the gap
// and the title floor; where that and the action fit on one line the group takes what is left, and
// where they do not the action wraps under it, still on the trailing edge.
const RowCard = ({ action, lead, meta, title }: RowCardProps) => (
  <Surface
    alignItems="center"
    backgroundColor="transparent"
    borderWidth={0}
    columnGap={GAP}
    flexDirection="row"
    flexWrap="wrap"
    minWidth={0}
    paddingHorizontal={12}
    paddingVertical={10}
    rowGap={4}
  >
    <Surface
      alignItems="center"
      backgroundColor="transparent"
      borderWidth={0}
      flexBasis={LEAD + GAP + TITLE_FLOOR}
      flexDirection="row"
      flexGrow={1}
      // A container narrower than the basis squeezes the words; it never overflows.
      flexShrink={1}
      gap={GAP}
      minWidth={0}
    >
      <Surface
        alignItems="center"
        backgroundColor="surfaceElevated"
        borderRadius="round"
        borderWidth={0}
        flexShrink={0}
        height={LEAD}
        justifyContent="center"
        width={LEAD}
      >
        {lead}
      </Surface>
      <Surface backgroundColor="transparent" borderWidth={0} flexBasis={0} flexGrow={1} gap={2} minWidth={0}>
        <Text as="p" textRole="cardTitle">
          {title}
        </Text>
        <CardMeta parts={meta} />
      </Surface>
    </Surface>
    {action ? (
      // The auto margin holds the action on the trailing edge of whichever line it lands on.
      <Surface backgroundColor="transparent" borderWidth={0} flexShrink={0} marginLeft="auto">
        {action}
      </Surface>
    ) : null}
  </Surface>
);

export { RowCard };
