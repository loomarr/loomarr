// PROTOTYPE — throwaway (#2151 step 2). Variant C · Collections grid and the collection-edit page.
import { Action, Badge, Field, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { COLLECTION_EDIT, COLLECTIONS, type CollectionRow } from "./prototype-data";
import type { Screen } from "./prototype-state";
import { Art, Crumbs, SectionLabel } from "./variant-c-parts";

const Card = ({ c, onEdit }: { c: CollectionRow; onEdit: () => void }) => {
  const tmdb = c.kind === "TMDB";
  return (
    <Surface gap="$3" padding="$4">
      <div className="flex items-start gap-3">
        <Art hue={c.hue} title={c.name} width={44} />
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <Text textRole="label">{c.name}</Text>
          {tmdb ? (
            <>
              <Text textRole="metadata" tone="secondary">
                {c.owned} of {c.total} owned
              </Text>
              <ProgressTrack
                label={`${c.name} owned`}
                percent={Math.round((c.owned / (c.total ?? 1)) * 100)}
              />
            </>
          ) : (
            <Text textRole="metadata" tone="secondary">
              {c.owned} titles
            </Text>
          )}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {c.fromEmby ? <Badge tone="info">from Emby</Badge> : null}
        {c.usedBy?.map((u) => (
          <Badge key={u} tone="signal">
            used by {u}
          </Badge>
        ))}
        {tmdb ? <Badge tone="neutral">read-only</Badge> : null}
      </div>
      <div className="flex flex-wrap gap-2">
        {tmdb ? (
          <Action variant="secondary">Copy to mine</Action>
        ) : (
          <Action onPress={onEdit} variant="secondary">
            Edit
          </Action>
        )}
      </div>
    </Surface>
  );
};

const CollectionsPage = ({ go }: { go: (s: Screen) => void }) => {
  const tmdb = COLLECTIONS.filter((c) => c.kind === "TMDB");
  const mine = COLLECTIONS.filter((c) => c.kind === "Mine");
  return (
    <div className="flex flex-col gap-6">
      <section aria-label="Your collections" className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <SectionLabel>Yours · editable</SectionLabel>
          <div className="flex flex-wrap gap-2">
            <Action icon="plus" onPress={() => go("collection-edit")} tone="primary">
              New collection
            </Action>
            <Action variant="outline">Copy from a TMDB collection</Action>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {mine.map((c) => (
            <Card c={c} key={c.id} onEdit={() => go("collection-edit")} />
          ))}
        </div>
      </section>
      <section aria-label="TMDB collections" className="flex flex-col gap-3">
        <SectionLabel>TMDB · read-only</SectionLabel>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {tmdb.map((c) => (
            <Card c={c} key={c.id} onEdit={() => go("collection-edit")} />
          ))}
        </div>
      </section>
    </div>
  );
};

const CollectionEditPage = ({ go }: { go: (s: Screen) => void }) => {
  const e = COLLECTION_EDIT;
  const [name, setName] = useState(e.name);
  const [desc, setDesc] = useState(e.description);
  const [add, setAdd] = useState("");
  const used = e.usedBy.length > 0;
  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <Crumbs onBack={() => go("collections")} trail={["Collections", name]} />
      <Surface gap="$3" padding="$4">
        <Field label="Name" onChangeText={setName} value={name} />
        <Field label="Description" onChangeText={setDesc} value={desc} />
      </Surface>
      <Surface gap="$2" padding="$4">
        <SectionLabel>Members · in order</SectionLabel>
        {e.members.map((m, i) => (
          <div className="border-border flex items-center gap-3 border-b py-2" key={m.title}>
            <span aria-hidden="true">
              <Text textRole="data" tone="muted">
                ⠿
              </Text>
            </span>
            <Text textRole="data" tone="muted">
              {i + 1}
            </Text>
            <Art hue={m.hue} title={m.title} width={24} />
            <div className="min-w-0 flex-1">
              <Text textRole="compact">{m.title}</Text>
            </div>
            <Text textRole="data" tone="secondary">
              {m.year}
            </Text>
            <Action variant="ghost">Remove</Action>
          </div>
        ))}
        <Field label="Add a title" onChangeText={setAdd} placeholder="Search your library" value={add} />
      </Surface>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-col gap-1">
          <Action disabled={used} tone="danger" variant="destructive">
            Delete collection
          </Action>
          <Text textRole="metadata" tone="muted">
            {used
              ? `Used by ${e.usedBy.join(", ")}. Remove it from the channel first.`
              : "Not used by any channel."}
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

export { CollectionEditPage, CollectionsPage };
