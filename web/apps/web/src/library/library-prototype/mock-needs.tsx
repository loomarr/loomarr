// PROTOTYPE — throwaway (#4476 A1). Needs you as its own page: files grouped by the folder they sit
// in, one card per title, the file name leading, the reasons in one sentence. Ignore (one file or
// a whole folder) has an undo and an Ignored tab to find it again.
import { Action, Icon, icons, Text, TextLink } from "@loomarr/design-system";
import { useState } from "react";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { IGNORED, NEEDS, type NeedsItem } from "./mock-data";
import { FileName, PosterArt, UnderlineTabs } from "./mock-parts";

type NeedsState = "several" | "clear" | "undo" | "folder-undo" | "ignored";

// A file with no guess has no poster to show, so it gets the file glyph, not an empty frame.
const Lead = ({ item }: { item: NeedsItem }) => (
  <div className="w-10 shrink-0 sm:w-14">
    {item.guess ? (
      <PosterArt small title={{ ...item.guess }} />
    ) : (
      <div className="bg-static-800 flex aspect-[2/3] w-full items-center justify-center rounded-md">
        <Icon decorative glyph={icons.noImage} size="compact" tone="muted" />
      </div>
    )}
  </div>
);

const Answers = ({ item, phone, onIgnore }: { item: NeedsItem; phone: boolean; onIgnore: () => void }) => (
  <div className="mt-1 flex flex-wrap items-center gap-2">
    {item.guess ? (
      <>
        <Action variant="outline">Yes, that's it</Action>
        <Action variant="ghost">Pick a different title</Action>
      </>
    ) : (
      <>
        <Action variant="outline">Pick the title</Action>
        <Action variant="ghost">It's ours, keep it as is</Action>
      </>
    )}
    {phone ? null : (
      <Action onPress={onIgnore} variant="ghost">
        Ignore this
      </Action>
    )}
  </div>
);

// On a phone the answers get the card's full width, and "Ignore this" sits in the card's corner, so no
// answer is ever left alone on a row.
const Card = ({ item, phone, onIgnore }: { item: NeedsItem; phone: boolean; onIgnore: () => void }) => (
  <li className="bg-static-900 flex flex-col gap-3 rounded-lg p-3 sm:p-4">
    <div className="flex gap-3">
      <Lead item={item} />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <div className="flex items-start justify-between gap-2">
          <FileName name={item.file} />
          {phone ? (
            <span className="-mt-2 -mr-2 shrink-0 whitespace-nowrap">
              <Action icon="conceal" onPress={onIgnore} variant="ghost">
                Ignore this
              </Action>
            </span>
          ) : null}
        </div>
        {item.extraFiles ? (
          <Text as="p" textRole="compact" tone="secondary">
            With {item.extraFiles} more parts of the same disc
          </Text>
        ) : null}
        <Text as="p" textRole="compact">
          {item.guess ? (
            <>
              Is this <strong>{item.guess.name}</strong> ({item.guess.year})? {item.guess.why[0]?.toUpperCase()}
              {item.guess.why.slice(1)}.
            </>
          ) : (
            item.none
          )}
        </Text>
        {phone ? null : <Answers item={item} onIgnore={onIgnore} phone={false} />}
      </div>
    </div>
    {phone ? <Answers item={item} onIgnore={onIgnore} phone /> : null}
  </li>
);

const IgnoredList = () => (
  <ul className="divide-static-700 flex flex-col divide-y">
    {IGNORED.map((i) => (
      <li className="flex items-center justify-between gap-3 py-3" key={i.id}>
        <div className="flex min-w-0 flex-col gap-0.5">
          <FileName name={i.file} />
          <Text as="p" numberOfLines={1} textRole="compact" tone="secondary">
            {i.folder} · {i.when}
          </Text>
        </div>
        <Action variant="outline">Bring it back</Action>
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

const NeedsYou = ({ state, phone, onBack }: { state: NeedsState; phone: boolean; onBack: () => void }) => {
  const [tab, setTab] = useState<"waiting" | "ignored">(state === "ignored" ? "ignored" : "waiting");
  const [ignored, setIgnored] = useState<string[][]>(
    state === "undo" ? [["n3"]] : state === "folder-undo" ? [["n1", "n6"]] : [],
  );
  const gone = ignored.flat();
  const waiting = state === "clear" ? [] : NEEDS.filter((n) => !gone.includes(n.id));
  const folders = [...new Set(waiting.map((n) => n.folder))];
  const last = ignored.at(-1);
  const lastItems = NEEDS.filter((n) => last?.includes(n.id));

  return (
    <div className="flex flex-col">
      <div className="px-6 pt-3">
        <Action icon="back" onPress={onBack} variant="ghost">
          Library
        </Action>
      </div>
      <PageHeader description="Files Loomarr couldn't name on its own. Answer once and it remembers." title="Needs you" />
      <div className="flex w-full max-w-4xl flex-col gap-4 px-6 pt-2 pb-28">
        <UnderlineTabs
          label="Which files"
          onChange={setTab}
          options={[
            { value: "waiting", label: "Waiting", count: String(waiting.length) },
            { value: "ignored", label: "Ignored", count: String(IGNORED.length + gone.length) },
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
          folders.map((folder) => {
            const items = waiting.filter((n) => n.folder === folder);
            return (
              <section aria-label={folder} className="flex flex-col gap-2" key={folder}>
                <div className="flex min-h-9 items-center justify-between gap-3">
                  <Text as="h2" numberOfLines={1} strong textRole="compact" tone="secondary">
                    {folder}
                  </Text>
                  {items.length > 1 ? (
                    <Action onPress={() => setIgnored((xs) => [...xs, items.map((n) => n.id)])} variant="ghost">
                      {phone ? `Ignore all ${items.length}` : `Ignore everything in this folder (${items.length})`}
                    </Action>
                  ) : null}
                </div>
                <ul className="flex flex-col gap-2">
                  {items.map((n) => (
                    <Card item={n} key={n.id} onIgnore={() => setIgnored((xs) => [...xs, [n.id]])} phone={phone} />
                  ))}
                </ul>
              </section>
            );
          })
        )}
      </div>
      {last && lastItems.length > 0 ? (
        <div
          className="bg-static-800 fixed inset-x-4 bottom-24 z-20 mx-auto flex max-w-md items-center gap-3 rounded-lg p-3 shadow-lg sm:bottom-6"
          role="status"
        >
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            {lastItems.length === 1 && lastItems[0] ? (
              <span className="flex min-w-0 items-baseline gap-1">
                <Text textRole="compact">Ignored</Text>
                <FileName name={lastItems[0].file} strong={false} />
              </span>
            ) : (
              <Text as="p" textRole="compact">
                Ignored {lastItems.length} files in {lastItems[0]?.folder}
              </Text>
            )}
            <Text as="p" textRole="compact" tone="secondary">
              Find them again under Ignored.
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
