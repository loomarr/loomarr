// PROTOTYPE — throwaway (#2151 step 2). Variant A: Collections table + collection editor (decision 28).
import { Action, Badge, Field, ProgressTrack, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { Pressable } from "react-native";
import type { CollectionRow } from "./prototype-data";
import { COLLECTION_EDIT, COLLECTIONS } from "./prototype-data";
import { Art, SectionLabel } from "./variant-a-parts";

const TmdbRow = ({ c }: { c: CollectionRow }) => (
  <Surface
    alignItems="center"
    flexDirection="row"
    flexWrap="wrap"
    gap="control"
    padding={8}
    paddingHorizontal="control"
  >
    <Art hue={c.hue} width={28} />
    <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={160}>
      <Text numberOfLines={1} textRole="label" tone="primary">
        {c.name}
      </Text>
    </Surface>
    <Surface backgroundColor="transparent" borderWidth={0} gap={4} width={140}>
      <Text textRole="data">{`${c.owned} of ${c.total} owned`}</Text>
      <ProgressTrack label={`${c.name} owned`} percent={Math.round((c.owned / (c.total ?? 1)) * 100)} />
    </Surface>
    <Badge>TMDB · read-only</Badge>
    <Action tone="secondary">Copy to yours</Action>
  </Surface>
);

const MineRow = ({ c, onEdit }: { c: CollectionRow; onEdit: () => void }) => (
  <Pressable accessibilityLabel={`Edit ${c.name}`} accessibilityRole="button" onPress={onEdit}>
    <Surface
      alignItems="center"
      flexDirection="row"
      flexWrap="wrap"
      gap="control"
      padding={8}
      paddingHorizontal="control"
    >
      <Art hue={c.hue} width={28} />
      <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={160}>
        <Text numberOfLines={1} textRole="label" tone="primary">
          {c.name}
        </Text>
        <Text textRole="metadata">
          {c.usedBy?.length ? `used by ${c.usedBy.join(", ")}` : "not used by a channel"}
        </Text>
      </Surface>
      <Text textRole="data">{`${c.owned} titles`}</Text>
      {c.fromEmby ? <Badge tone="info">from Emby</Badge> : null}
      <Action icon="disclosure" onPress={onEdit} tone="secondary">
        Edit
      </Action>
    </Surface>
  </Pressable>
);

const CollectionsTable = ({ onEdit }: { onEdit: () => void }) => (
  <Surface backgroundColor="transparent" borderWidth={0} gap="section">
    <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" flexWrap="wrap" gap="inline">
      <Action icon="plus" tone="primary">
        New collection
      </Action>
      <Action tone="secondary">Copy from a TMDB collection</Action>
    </Surface>
    <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
      <SectionLabel>From TMDB</SectionLabel>
      {COLLECTIONS.filter((c) => c.kind === "TMDB").map((c) => (
        <TmdbRow c={c} key={c.id} />
      ))}
    </Surface>
    <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
      <SectionLabel>Yours</SectionLabel>
      {COLLECTIONS.filter((c) => c.kind === "Mine").map((c) => (
        <MineRow c={c} key={c.id} onEdit={onEdit} />
      ))}
    </Surface>
  </Surface>
);

const CollectionEditor = ({ onDone }: { onDone: () => void }) => {
  const [name, setName] = useState(COLLECTION_EDIT.name);
  const [description, setDescription] = useState(COLLECTION_EDIT.description);
  const [members, setMembers] = useState(COLLECTION_EDIT.members);
  const [query, setQuery] = useState("");
  const used = COLLECTION_EDIT.usedBy.length > 0;

  return (
    <>
      <Surface backgroundColor="transparent" borderWidth={0} gap="control" padding="control">
        <Field label="Name" onChangeText={setName} value={name} />
        <Field label="Description" multiline onChangeText={setDescription} value={description} />
        <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
          <SectionLabel>{`Members · ${members.length}, in order`}</SectionLabel>
          {members.map((m, i) => (
            <Surface
              alignItems="center"
              flexDirection="row"
              gap="control"
              key={m.title}
              padding={6}
              paddingHorizontal="control"
            >
              <Text aria-label="Drag to reorder" textRole="label">
                ⠿
              </Text>
              <div className="w-5 shrink-0">
                <Text textRole="data">{String(i + 1)}</Text>
              </div>
              <Art hue={m.hue} width={24} />
              <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
                <Text numberOfLines={1} textRole="compact" tone="primary">
                  {`${m.title} (${m.year})`}
                </Text>
              </Surface>
              <Action
                accessibilityLabel={`Remove ${m.title}`}
                icon="close"
                onPress={() => setMembers((cur) => cur.filter((x) => x.title !== m.title))}
                variant="ghost"
              >
                Remove
              </Action>
            </Surface>
          ))}
        </Surface>
        <Field
          accessibilityLabel="Add a member"
          label="Add a member"
          onChangeText={setQuery}
          placeholder="Search your library by title"
          value={query}
        />
      </Surface>
      <Surface
        backgroundColor="transparent"
        borderRadius={0}
        borderWidth={0}
        borderTopWidth={1}
        flexDirection="row"
        flexWrap="wrap"
        gap="inline"
        padding="control"
      >
        <Action onPress={onDone} tone="primary">
          Save
        </Action>
        <Action onPress={onDone} variant="outline">
          Cancel
        </Action>
        <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={160}>
          <Action disabled={used} tone="danger">
            Delete collection
          </Action>
          <Text textRole="metadata">
            {used
              ? `In use by ${COLLECTION_EDIT.usedBy.join(", ")} — remove it there first.`
              : "Not used by any channel."}
          </Text>
        </Surface>
      </Surface>
    </>
  );
};

export { CollectionEditor, CollectionsTable };
