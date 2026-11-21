// PROTOTYPE — throwaway (#4476 A1). Needs you as its own page: one card per title, the file name
// leading, the reasons in one sentence, Ignore with an undo and a place to find it again.
import { Action, Icon, icons, Text, TextLink } from "@loomarr/design-system";
import { useState } from "react";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { IGNORED, NEEDS, type NeedsItem } from "./mock-data";
import { PosterArt, Proposal, UnderlineTabs } from "./mock-parts";

type NeedsState = "several" | "clear" | "undo" | "ignored" | "proposals";

// A file with no guess has no poster to show, so it gets the file glyph, not an empty frame.
const Lead = ({ item }: { item: NeedsItem }) => (
  <div className="w-12 shrink-0 sm:w-16">
    {item.guess ? (
      <PosterArt small title={{ name: item.guess.name, year: item.guess.year, hue: item.guess.hue, layout: item.guess.layout }} />
    ) : (
      <div className="bg-static-800 text-static-400 flex aspect-[2/3] w-full items-center justify-center rounded-md">
        <Icon decorative glyph={icons.noImage} tone="muted" />
      </div>
    )}
  </div>
);

const Card = ({
  item,
  selecting,
  oursProposal,
  onIgnore,
}: {
  item: NeedsItem;
  selecting: boolean;
  oursProposal: boolean;
  onIgnore: () => void;
}) => (
  <li className="bg-static-900 flex gap-3 rounded-lg p-3 sm:gap-4 sm:p-4">
    {selecting ? (
      <input aria-label={`Select ${item.file}`} className="accent-static-100 mt-1 size-4 shrink-0" defaultChecked={item.folder === "Movies/Unsorted"} type="checkbox" />
    ) : null}
    <Lead item={item} />
    <div className="flex min-w-0 flex-1 flex-col gap-1.5">
      <span className="break-all">
        <Text strong textRole="data">
          {item.file}
        </Text>
      </span>
      <span className="break-all">
        <Text as="p" textRole="metadata" tone="secondary">
          in {item.folder}
          {item.extraFiles ? ` · with ${item.extraFiles} more parts of the same disc` : ""}
        </Text>
      </span>
      <Text as="p" textRole="body">
        {item.guess ? (
          <>
            Is this <strong>{item.guess.name}</strong> ({item.guess.year})? {item.guess.why[0]?.toUpperCase()}
            {item.guess.why.slice(1)}.
          </>
        ) : (
          item.none
        )}
      </Text>
      <div className="mt-1.5 flex flex-wrap items-center gap-2">
        {item.guess ? (
          <>
            <Action variant="outline">Yes, that's it</Action>
            <Action variant="ghost">Pick a different title</Action>
          </>
        ) : (
          <Action variant="outline">Pick the title</Action>
        )}
        <Action onPress={onIgnore} variant="ghost">
          Ignore this
        </Action>
      </div>
      {oursProposal && !item.guess ? (
        <Proposal className="mt-2" question="A third answer for a file that's in no catalogue">
          <div className="flex flex-col items-start gap-2">
            <Action icon="success" variant="outline">
              It's ours, keep it as is
            </Action>
            <Text as="p" textRole="metadata" tone="secondary">
              Keeps the file's own name, files it under Home videos, and never looks it up again.
            </Text>
          </div>
        </Proposal>
      ) : null}
    </div>
  </li>
);

const IgnoredList = () => (
  <ul className="divide-static-700 flex flex-col divide-y">
    {IGNORED.map((i) => (
      <li className="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between" key={i.id}>
        <div className="min-w-0">
          <span className="break-all">
            <Text strong textRole="data">
              {i.file}
            </Text>
          </span>
          <Text as="p" textRole="metadata" tone="secondary">
            in {i.folder} · {i.when}
          </Text>
        </div>
        <div className="flex gap-2">
          <Action variant="outline">Bring it back</Action>
        </div>
      </li>
    ))}
  </ul>
);

const AllCaughtUp = ({ onBack }: { onBack: () => void }) => (
  <div className="flex flex-col items-start gap-3 py-12 sm:items-center sm:py-20 sm:text-center">
    <span className="text-lock">
      <Icon decorative glyph={icons.success} size="tv" tone="inherit" />
    </span>
    <Text as="h2" textRole="heading">
      All caught up
    </Text>
    <Text as="p" textRole="body" tone="secondary">
      Loomarr named every file it found. When it can't name a new one, it waits here.
    </Text>
    <div className="mt-2 flex flex-wrap items-center gap-4">
      <Action icon="back" onPress={onBack} tone="primary">
        Back to the Library
      </Action>
      <TextLink tone="info">3 ignored files</TextLink>
    </div>
  </div>
);

const NeedsYou = ({ state, onBack }: { state: NeedsState; onBack: () => void }) => {
  const [tab, setTab] = useState<"waiting" | "ignored">(state === "ignored" ? "ignored" : "waiting");
  const [ignored, setIgnored] = useState<string[]>(state === "undo" ? ["n3"] : []);
  const proposals = state === "proposals";
  const waiting = state === "clear" ? [] : NEEDS.filter((n) => !ignored.includes(n.id));
  const lastIgnored = NEEDS.find((n) => n.id === ignored.at(-1));

  return (
    <div className="flex flex-col">
      <div className="px-6 pt-3">
        <Action icon="back" onPress={onBack} variant="ghost">
          Library
        </Action>
      </div>
      <PageHeader
        description="Files Loomarr couldn't name on its own. Answer once and it remembers."
        title="Needs you"
      />
      <div className="flex w-full max-w-4xl flex-col gap-4 px-6 pt-2 pb-24">
        <UnderlineTabs
          label="Which files"
          onChange={setTab}
          options={[
            { value: "waiting", label: "Waiting", count: String(waiting.length) },
            { value: "ignored", label: "Ignored", count: String(IGNORED.length + ignored.length) },
          ]}
          value={tab}
        />
        {tab === "ignored" ? (
          <>
            <Text as="p" textRole="compact" tone="secondary">
              Ignored files stay where they are. Bring one back and it waits here again.
            </Text>
            <IgnoredList />
          </>
        ) : waiting.length === 0 ? (
          <AllCaughtUp onBack={onBack} />
        ) : (
          <>
            {proposals ? (
              <Proposal question="Answering several at once">
                <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <Text textRole="compact">2 selected, both in Movies/Unsorted</Text>
                  <div className="flex flex-wrap gap-2">
                    <Action variant="outline">Ignore these 2</Action>
                    <Action variant="ghost">Ignore everything in Movies/Unsorted</Action>
                  </div>
                </div>
              </Proposal>
            ) : null}
            <ul className="flex flex-col gap-3">
              {waiting.map((n) => (
                <Card
                  item={n}
                  key={n.id}
                  onIgnore={() => setIgnored((xs) => [...xs, n.id])}
                  oursProposal={proposals}
                  selecting={proposals}
                />
              ))}
            </ul>
          </>
        )}
      </div>
      {lastIgnored ? (
        <div
          className="bg-static-800 fixed inset-x-4 bottom-24 z-20 mx-auto flex max-w-md items-center gap-3 rounded-lg p-3 shadow-lg sm:bottom-6"
          role="status"
        >
          <div className="min-w-0 flex-1">
            <Text as="p" textRole="compact">Ignored {lastIgnored.file}.</Text>
            <Text as="p" textRole="metadata" tone="secondary">
              Find it again under Ignored.
            </Text>
          </div>
          <Action onPress={() => setIgnored((xs) => xs.slice(0, -1))} variant="outline">
            Undo
          </Action>
        </div>
      ) : null}
    </div>
  );
};

export type { NeedsState };
export { NeedsYou };
