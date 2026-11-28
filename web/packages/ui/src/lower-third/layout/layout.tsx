import { Surface } from "@loomarr/design-system";
import type { ComponentProps } from "react";

/** A part of the lower third that only lays out: no fill, edge or corner. */
const Box = (props: ComponentProps<typeof Surface>) => (
  <Surface backgroundColor="transparent" borderRadius={0} borderWidth={0} {...props} />
);

/** One row of the words under the title. */
const Line = (props: ComponentProps<typeof Surface>) => (
  <Box alignItems="center" flexDirection="row" gap={8} marginTop={6} minWidth={0} {...props} />
);

export { Box, Line };
