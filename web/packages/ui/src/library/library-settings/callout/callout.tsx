import { Surface, Text } from "@loomarr/design-system";
import type { ReactNode } from "react";

import { Line } from "../../library-poster";

// A calm warning with one way out. It stands for as long as the state lasts, so it is a polite
// status, not an alert that interrupts the reader each time the page draws it.
const Callout = ({
  title,
  children,
  actions,
}: {
  title: string;
  children?: ReactNode;
  actions?: ReactNode;
}) => (
  <Surface borderColor="stateWarning" borderRadius="control" gap="control" padding="section" role="status">
    <Text textRole="headline" tone="warning">
      {title}
    </Text>
    {children ? (
      <Text textRole="body" tone="secondary">
        {children}
      </Text>
    ) : null}
    {actions ? <Line gap="control">{actions}</Line> : null}
  </Surface>
);

export { Callout };
