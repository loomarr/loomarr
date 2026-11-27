import { Surface, Text } from "@loomarr/design-system";

import type { LibraryNeedsCardsProps } from "../library-needs.type";
import { NeedCard } from "../need-card";
import { OrderCard } from "../order-card";
import { ProblemCard } from "../problem-card";
import { problemEntries } from "../problem-entries";
import { useAnswers } from "../use-answers";
import { useOrderAnswers } from "../use-order-answers";

const LibraryNeedsCards = ({
  needs,
  orders,
  problems,
  onLink,
  onIgnore,
  onChooseOrder,
  onSearch,
  onOpenTitle,
}: LibraryNeedsCardsProps) => {
  const { list: open, phases, failed, answer, done } = useAnswers(needs);
  const shows = useOrderAnswers(orders, onChooseOrder);
  const entries = problemEntries(problems);
  const waiting =
    shows.list.filter((n) => !shows.phases[n.id]).length +
    open.filter((n) => !phases[n.id]).length +
    entries.length;
  return (
    <Surface aria-label="Needs you" backgroundColor="transparent" borderWidth={0} gap="control" role="region">
      <Text as="h2" textRole="headline">{`Needs you (${waiting})`}</Text>
      {shows.list.length + open.length + entries.length === 0 ? (
        <Text textRole="body" tone="secondary">
          Nothing needs you.
        </Text>
      ) : null}
      {shows.list.map((need, i) => (
        <OrderCard
          done={shows.doneOf(need.id)}
          failed={shows.failed[need.id]}
          key={need.id}
          need={need}
          onChoose={(option) => shows.choose(need, option)}
          onDone={() => shows.done(need.id)}
          onOpenTitle={onOpenTitle}
          primary={i === 0}
        />
      ))}
      {open.map((need, i) => (
        <NeedCard
          key={need.id}
          need={need}
          onDone={() => done(need.id)}
          onIgnore={() => answer(need.id, "ignored", onIgnore(need.id))}
          onPick={(c) => answer(need.id, "added", onLink(need.id, c.externalId))}
          onSearch={onSearch}
          onYes={() => {
            if (need.best) answer(need.id, "added", onLink(need.id, need.best.externalId));
          }}
          failed={failed[need.id]}
          phase={phases[need.id]}
          primary={shows.list.length === 0 && i === 0}
        />
      ))}
      {entries.map((entry) => (
        <ProblemCard entry={entry} key={entry.key} onOpenTitle={onOpenTitle} />
      ))}
    </Surface>
  );
};

export { LibraryNeedsCards };
