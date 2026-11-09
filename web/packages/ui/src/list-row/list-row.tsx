import { ProgressTrack, StatusDot, Surface, Text } from "@loomarr/design-system";
import { Children } from "react";

import type { ListGroupProps, ListRowProps } from "./list-row.type";

// ListGroup + ListRow — the bordered list of state rows (#1659 web mock: On the way, Your
// requests). A dot for what the row is waiting on, a title and sub line, and at most one of a
// progress readout or a trailing action.
//
// A real list, so a screen reader announces "list, 3 items" before reading rows that otherwise only
// look like one. The group owns each item and the hairline BETWEEN items: the mock's line under the
// last row doubles against the group's own border, so it is not drawn.
const ListGroup = ({ children, framed = true, ...aria }: ListGroupProps) => (
  <Surface
    {...aria}
    {...(framed ? {} : { backgroundColor: "transparent", borderRadius: 0, borderWidth: 0 })}
    overflow="hidden"
    padding={0}
    role="list"
  >
    {Children.toArray(children).map((child, index) => (
      <Surface
        backgroundColor="transparent"
        borderColor="borderDecorative"
        borderRadius={0}
        borderWidth={0}
        borderTopWidth={index === 0 ? 0 : 1}
        key={(child as { key?: string | null }).key ?? index}
        role="listitem"
      >
        {child}
      </Surface>
    ))}
  </Surface>
);

const ListRow = ({ action, progress, sub, title, tone, toneLabel = "" }: ListRowProps) => (
  <Surface
    alignItems="center"
    backgroundColor="transparent"
    borderWidth={0}
    flexDirection="row"
    gap={16}
    paddingHorizontal={16}
    paddingVertical={12}
  >
    <StatusDot label={toneLabel} tone={tone} />
    <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap={2} minWidth={0}>
      <Text strong textRole="body">
        {title}
      </Text>
      {sub != null && (
        <Text textRole="caption" tone="secondary">
          {sub}
        </Text>
      )}
    </Surface>
    {progress && (
      <Surface backgroundColor="transparent" borderWidth={0} flexShrink={0} gap={4} width={160}>
        <ProgressTrack label={progress.label} percent={progress.value} tone="tune" width="100%" />
        {progress.eta && (
          <Text textAlign="right" textRole="mono">
            {progress.eta}
          </Text>
        )}
      </Surface>
    )}
    {action}
  </Surface>
);

export { ListGroup, ListRow };
