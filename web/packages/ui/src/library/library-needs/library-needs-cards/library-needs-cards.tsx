import { Action, Surface, Text, useWindowWidth } from "@loomarr/design-system";
import type { ReactNode } from "react";

import { Line } from "../../library-poster";
import type { LibraryNeedsCardsProps } from "../library-needs.type";
import { NeedCard } from "../need-card";
import { OrderCard } from "../order-card";
import { ProblemCard } from "../problem-card";
import { problemEntries } from "../problem-entries";
import { useAnswers } from "../use-answers";
import { useOrderAnswers } from "../use-order-answers";
import { waitingTitles } from "../waiting-titles";

// One group under its heading; `action` sits at the heading's end.
const Group = ({
  label,
  heading,
  action,
  children,
}: {
  label: string;
  heading: string;
  action?: ReactNode;
  children: ReactNode;
}) => (
  <Surface aria-label={label} backgroundColor="transparent" borderWidth={0} gap="control" role="region">
    <Line justifyContent="space-between">
      <Text as="h3" textRole="title">
        {heading}
      </Text>
      {action}
    </Line>
    {children}
  </Surface>
);

const LibraryNeedsCards = ({
  needs,
  orders,
  problems,
  hideHeading,
  restored,
  empty,
  onLink,
  onIgnore,
  onIgnoreFolder,
  onKeep,
  onChooseOrder,
  onSearch,
  onOpenTitle,
}: LibraryNeedsCardsProps) => {
  const { list: open, phases, failed, answer, done } = useAnswers(needs, restored);
  const wide = useWindowWidth() >= 640;
  const shows = useOrderAnswers(orders, onChooseOrder);
  const entries = problemEntries(problems);
  // One card per title: a question's card lists what else is wrong with it.
  const problemOf = new Map(entries.map((entry) => [entry.id, entry]));
  const asked = new Set([...shows.list, ...open].map((n) => n.id));
  const alone = entries.filter((entry) => !asked.has(entry.id));
  const waiting = waitingTitles(
    shows.list.filter((n) => !shows.phases[n.id]),
    open.filter((n) => !phases[n.id]),
    entries,
  );
  const folders = [...new Set(open.map((need) => need.folder ?? ""))];

  // Exactly the cards still asking under the heading, in one save.
  const ignoreFolder = (folder: string, ids: string[]) => {
    const save = onIgnoreFolder?.(folder, ids);
    if (save) for (const id of ids) answer(id, "ignored", save);
  };

  return (
    <Surface aria-label="Needs you" backgroundColor="transparent" borderWidth={0} gap="control" role="region">
      {hideHeading ? null : <Text as="h2" textRole="headline">{`Needs you (${waiting})`}</Text>}
      {shows.list.length + open.length + entries.length === 0
        ? (empty ?? (
            <Text textRole="body" tone="secondary">
              Nothing needs you.
            </Text>
          ))
        : null}
      {folders.map((folder, group) => {
        const cards = open.filter((need) => (need.folder ?? "") === folder);
        const asking = cards.filter((need) => !phases[need.id]).map((need) => need.id);
        const list = cards.map((need, i) => (
          <NeedCard
            key={`${need.id}:${restored?.[need.id] ?? 0}`}
            need={need}
            onDone={() => done(need.id)}
            onIgnore={() => answer(need.id, "ignored", onIgnore(need.id))}
            onKeep={onKeep ? () => answer(need.id, "added", onKeep(need.id)) : undefined}
            onOpenTitle={onOpenTitle}
            problem={problemOf.get(need.id)}
            onPick={(c) => answer(need.id, "added", onLink(need.id, c.externalId))}
            onSearch={onSearch}
            onYes={() => {
              if (need.best) answer(need.id, "added", onLink(need.id, need.best.externalId));
            }}
            failed={failed[need.id]}
            phase={phases[need.id]}
            primary={group === 0 && i === 0}
          />
        ));
        if (!folder) return list;
        return (
          <Group
            action={
              onIgnoreFolder && asking.length > 1 ? (
                <Action
                  density={wide ? "pointer" : "touch"}
                  onPress={() => ignoreFolder(folder, asking)}
                  variant="ghost"
                >
                  {`Ignore these ${asking.length}`}
                </Action>
              ) : null
            }
            heading={folder}
            key={folder}
            label={`Folder ${folder}`}
          >
            {list}
          </Group>
        );
      })}
      {shows.list.length > 0 ? (
        <Group heading="Episode order" label="Episode order questions">
          {shows.list.map((need, i) => (
            <OrderCard
              done={shows.doneOf(need.id)}
              failed={shows.failed[need.id]}
              key={need.id}
              need={need}
              onChoose={(option) => shows.choose(need, option)}
              onDone={() => shows.done(need.id)}
              onOpenTitle={onOpenTitle}
              primary={open.length === 0 && i === 0}
              problem={problemOf.get(need.id)}
            />
          ))}
        </Group>
      ) : null}
      {alone.length > 0 ? (
        <Group heading="Needs a look" label="Needs a look">
          {alone.map((entry) => (
            <ProblemCard entry={entry} key={entry.id} onOpenTitle={onOpenTitle} />
          ))}
        </Group>
      ) : null}
    </Surface>
  );
};

export { LibraryNeedsCards };
