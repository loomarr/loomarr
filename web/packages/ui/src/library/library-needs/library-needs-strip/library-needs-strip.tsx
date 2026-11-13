import { Action, Surface, Text } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";

import { Line, Stack } from "../../library-poster";
import type { LibraryNeedsStripProps } from "../library-needs.type";
import { OrderRow } from "../order-row";
import { problemEntries } from "../problem-entries";
import { ProblemRow } from "../problem-row";
import { StripRow } from "../strip-row";
import { useAnswers } from "../use-answers";

// NEEDS YOU (#2151 decision 27, design/library variant D): the titles Loomarr can't tell what they
// are. The strip on the Library page asks about the first four; Needs you holds one card each. The
// words are the mock's word sheet: "Yes, that's it", "Pick a different title", "Ignore this".

const STRIP_FIRST = 4;

const LibraryNeedsStrip = ({
  needs,
  orders,
  problems,
  onLink,
  onOpenNeeds,
  onOpenTitle,
}: LibraryNeedsStripProps) => {
  const { list: open, phases, failed, answer, done } = useAnswers(needs);
  const [all, setAll] = useState(false);
  const entries = problemEntries(problems);
  // The shows whose order to say, the titles to answer, then the ones that need a look, as the
  // mocks list them.
  const rows: { key: string; row: ReactNode }[] = [
    ...orders.map((need) => ({
      key: `order:${need.id}`,
      row: <OrderRow need={need} onOpenNeeds={onOpenNeeds} />,
    })),
    ...open.map((need) => ({
      key: need.id,
      row: (
        <StripRow
          answered={phases[need.id] !== undefined}
          failed={failed[need.id]}
          need={need}
          onDone={() => done(need.id)}
          onOpenNeeds={onOpenNeeds}
          onYes={() => {
            if (need.best) answer(need.id, "added", onLink(need.id, need.best.externalId));
          }}
        />
      ),
    })),
    ...entries.map((entry) => ({
      key: entry.key,
      row: <ProblemRow entry={entry} onOpenTitle={onOpenTitle} />,
    })),
  ];
  if (rows.length === 0) return null;
  const shown = all ? rows : rows.slice(0, STRIP_FIRST);
  const waiting = orders.length + open.filter((n) => !phases[n.id]).length + entries.length;
  return (
    <Surface aria-label="Needs you" borderColor="stateWarning" gap="inline" padding="control" role="region">
      <Text textRole="headline">{`Needs you (${waiting})`}</Text>
      <Stack>
        {shown.map(({ key, row }, i) => (
          <Stack borderColor="borderDecorative" borderTopWidth={i === 0 ? 0 : 1} key={key}>
            {row}
          </Stack>
        ))}
      </Stack>
      {rows.length > STRIP_FIRST && !all ? (
        <Line>
          <Action onPress={() => setAll(true)} variant="ghost">
            {`Show ${rows.length - STRIP_FIRST} more`}
          </Action>
        </Line>
      ) : null}
    </Surface>
  );
};

export { LibraryNeedsStrip };
