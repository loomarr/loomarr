import { Action, Text } from "@loomarr/design-system";

import { Line, Stack } from "../../library-poster";
import type { LibraryOrderNeed } from "../library-needs.type";
import { NeedThumb } from "../need-thumb";
import { onlyCatalog, orderLine } from "../order-options";

// The strip asks in one line; Choose opens Needs you, since the question has more than two answers.
const OrderRow = ({ need, onOpenNeeds }: { need: LibraryOrderNeed; onOpenNeeds: () => void }) => (
  <Line columnGap="control" flexWrap="wrap" paddingVertical="control" rowGap="inline">
    <Line flexBasis={240} flexGrow={1} flexWrap="nowrap" minWidth={0}>
      <NeedThumb art={need.poster} name={need.name} width={36} />
      <Stack flexShrink={1} minWidth={0}>
        <Text textRole="body">{orderLine(need)}</Text>
      </Stack>
    </Line>
    <Action onPress={onOpenNeeds} variant="outline">
      {onlyCatalog(need) ? "See why" : "Choose"}
    </Action>
  </Line>
);

export { OrderRow };
