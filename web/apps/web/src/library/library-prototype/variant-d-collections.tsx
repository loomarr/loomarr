// PROTOTYPE — throwaway (#2151 step 2). Variant D · Collections as stacks of posters, and a calm
// collection editor. Plain words per PLAIN-WORDS.md.
import { Action, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { COLLECTION_EDIT, COLLECTIONS, type CollectionRow } from "./prototype-data";
import type { Screen } from "./prototype-state";
import { BackLink, channelPhrase, Heading, Poster, TextBox, Thumb } from "./variant-d-parts";

// Three posters fanned out behind one another: a collection reads as a pile of real things.
const PosterStack = ({ hue, name }: { hue: number; name: string }) => (
  <div className="relative h-40 w-28 shrink-0">
    {[2, 1, 0].map((i) => (
      <div className="absolute w-24" key={i} style={{ left: i * 8, top: (2 - i) * 6 }}>
        <Poster hue={(hue + i * 24) % 360} title={name} />
      </div>
    ))}
  </div>
);

const CollectionCard = ({ c, onEdit }: { c: CollectionRow; onEdit: () => void }) => {
  const tmdb = c.kind === "TMDB";
  const [copied, setCopied] = useState(false);
  return (
    <Surface gap="control" padding="control">
      <div className="flex gap-4">
        <PosterStack hue={c.hue} name={c.name} />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <Text textRole="title">{c.name}</Text>
          {tmdb ? (
            <Text textRole="compact" tone="secondary">
              From TMDB — you have {c.owned} of {c.total}
            </Text>
          ) : (
            <Text textRole="compact" tone="secondary">
              {c.owned} titles
            </Text>
          )}
          {c.fromEmby ? (
            <Text textRole="compact" tone="muted">
              Brought over from Emby
            </Text>
          ) : null}
          {c.usedBy?.map((u) => (
            <Text key={u} textRole="compact" tone="muted">
              {channelPhrase(u)}
            </Text>
          ))}
        </div>
      </div>
      <div>
        {tmdb ? (
          <Action disabled={copied} onPress={() => setCopied(true)} variant="secondary">
            {copied ? "Copied — it's under Yours" : "Make my own copy"}
          </Action>
        ) : (
          <Action onPress={onEdit} variant="secondary">
            Edit
          </Action>
        )}
      </div>
    </Surface>
  );
};

const CollectionsShelf = ({ go }: { go: (s: Screen) => void }) => {
  const mine = COLLECTIONS.filter((c) => c.kind === "Mine");
  const tmdb = COLLECTIONS.filter((c) => c.kind === "TMDB");
  return (
    <div className="flex flex-col gap-8">
      <section aria-label="Yours" className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Heading>Yours</Heading>
          <Action icon="plus" onPress={() => go("collection-edit")} tone="primary">
            New collection
          </Action>
        </div>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {mine.map((c) => (
            <CollectionCard c={c} key={c.id} onEdit={() => go("collection-edit")} />
          ))}
        </div>
      </section>
      <section aria-label="From TMDB" className="flex flex-col gap-4">
        <Heading>Ready-made sets</Heading>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {tmdb.map((c) => (
            <CollectionCard c={c} key={c.id} onEdit={() => go("collection-edit")} />
          ))}
        </div>
      </section>
    </div>
  );
};

const CollectionEditor = ({ go }: { go: (s: Screen) => void }) => {
  const e = COLLECTION_EDIT;
  const [name, setName] = useState(e.name);
  const [desc, setDesc] = useState(e.description);
  const [add, setAdd] = useState("");
  const [members, setMembers] = useState(e.members);
  const used = e.usedBy.length > 0;
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-4 md:p-8">
      <BackLink current={name || "New collection"} label="Collections" onBack={() => go("collections")} />
      <Text textRole="display">{name || "New collection"}</Text>
      <Surface gap="control" padding="section">
        <TextBox label="Name" onChange={setName} value={name} />
        <TextBox label="Description" onChange={setDesc} value={desc} />
      </Surface>
      <Surface gap="control" padding="section">
        <Heading>In this collection</Heading>
        <Text textRole="compact" tone="secondary">
          Drag to change the order.
        </Text>
        <div className="flex flex-col gap-1">
          {members.map((m) => (
            <div className="border-border flex items-center gap-3 border-b py-2" key={m.title}>
              <span aria-hidden="true" className="cursor-grab select-none">
                <Text textRole="body" tone="muted">
                  ⠿
                </Text>
              </span>
              <Thumb hue={m.hue} title={m.title} width={36} />
              <div className="flex min-w-0 flex-1 flex-col">
                <Text textRole="body">{m.title}</Text>
                <Text textRole="compact" tone="secondary">
                  {m.year}
                </Text>
              </div>
              <Action onPress={() => setMembers(members.filter((x) => x.title !== m.title))} variant="ghost">
                Remove
              </Action>
            </div>
          ))}
        </div>
        <TextBox label="Add a title" onChange={setAdd} placeholder="Search your library" value={add} />
      </Surface>
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <Action disabled={used} tone="danger" variant="outline">
            Delete collection
          </Action>
          <Text textRole="compact" tone="muted">
            {used
              ? `${e.usedBy.map(channelPhrase).join(" and ")} uses this. Take it off the channel first.`
              : "No channel uses this collection."}
          </Text>
        </div>
        <div className="flex gap-2">
          <Action onPress={() => go("collections")} variant="ghost">
            Cancel
          </Action>
          <Action onPress={() => go("collections")} tone="primary">
            Save
          </Action>
        </div>
      </div>
    </div>
  );
};

export { CollectionEditor, CollectionsShelf };
