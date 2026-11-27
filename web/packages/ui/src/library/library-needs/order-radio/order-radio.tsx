import { Badge, RadioItem, Text } from "@loomarr/design-system";

import { Line, Stack } from "../../library-poster";
import type { OrderOption } from "../order-options";

const OrderRadio = ({ option }: { option: OrderOption }) => (
  <RadioItem label={option.name} ringRadius={0} value={option.key}>
    {({ checked }) => (
      <Line
        alignItems="flex-start"
        backgroundColor={checked ? "surfaceFocus" : "transparent"}
        borderRadius={0}
        columnGap="control"
        flexWrap="nowrap"
        paddingHorizontal={14}
        paddingVertical={12}
      >
        <Stack
          alignItems="center"
          borderColor={checked ? "actionPrimary" : "borderControl"}
          borderRadius="round"
          borderWidth={1.5}
          flexShrink={0}
          height={18}
          justifyContent="center"
          marginTop={1}
          width={18}
        >
          {checked ? (
            <Stack backgroundColor="actionPrimary" borderRadius="round" height={8} width={8} />
          ) : null}
        </Stack>
        <Stack flexShrink={1} gap={2} minWidth={0}>
          <Line columnGap="control" flexWrap="wrap" rowGap={4}>
            <Text strong textRole="body">
              {option.name}
            </Text>
            {option.fits ? <Badge tone="success">Fits your files</Badge> : null}
          </Line>
          <Text textRole="compact" tone="muted">
            {option.sub}
          </Text>
        </Stack>
      </Line>
    )}
  </RadioItem>
);

export { OrderRadio };
