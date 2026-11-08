import { Surface } from "@loomarr/design-system";

import type { PanelRowProps } from "./panel-row.type";

// PanelRow — the dashboard panels' row (Playout, Services, Activity), placed in a ListGroup that
// owns the list semantics and the hairlines between rows.
//
// It owns ONE contract: content WRAPS to a second line on a narrow viewport instead of being
// truncated or clipped. A hand-rolled row once gave its trailing cluster a fixed width with no
// minWidth 0 on the row, and the card's overflow clipped it — visibly in Firefox, which resolves
// flex `min-width: auto` differently from Chromium. minWidth 0 is load-bearing on both the row and
// the main region; flexBasis 256 is what drops the meta cluster to its own line once the row is
// narrower than identity + cluster. A cluster wider than a phone row wraps within itself (#1785).
const PanelRow = ({ children, meta }: PanelRowProps) => (
  <Surface
    alignItems="center"
    backgroundColor="transparent"
    borderWidth={0}
    columnGap={16}
    flexDirection="row"
    flexWrap="wrap"
    justifyContent="space-between"
    minWidth={0}
    paddingHorizontal={16}
    paddingVertical={12}
    rowGap={8}
  >
    <Surface
      backgroundColor="transparent"
      borderWidth={0}
      flexBasis={256}
      flexGrow={1}
      flexShrink={1}
      minWidth={0}
    >
      {children}
    </Surface>
    {meta != null && (
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        flexShrink={0}
        flexWrap="wrap"
        gap={12}
        maxWidth="100%"
      >
        {meta}
      </Surface>
    )}
  </Surface>
);

export { PanelRow };
