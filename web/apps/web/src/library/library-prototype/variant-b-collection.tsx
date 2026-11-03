// PROTOTYPE — throwaway (#2151 step 2). Variant B right pane: collection detail and editor
// (decision 28). TMDB collections are read-only with an owned count; Loomarr's are editable.
import { Action, Badge, Field, ProgressTrack, Text } from "@loomarr/design-system";
import { useState } from "react";
import { COLLECTION_EDIT, type CollectionRow } from "./prototype-data";
import { Art, Box, Meta, Panel, SectionLabel } from "./variant-b-parts";

const CollectionHeader = ({ row }: { row: CollectionRow }) => (
  <Box alignItems="flex-start" flexDirection="row" gap="control">
    <Art hue={row.hue} title={row.name} width={56} />
    <Box flex={1} gap="inline" minWidth={0}>
      <Text textRole="title">{row.name}</Text>
      <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline">
        <Badge tone="neutral">{row.kind === "TMDB" ? "TMDB · read-only" : "Loomarr · editable"}</Badge>
        {row.fromEmby ? <Badge tone="neutral">from Emby</Badge> : null}
        <Meta>{row.total ? `${row.owned} of ${row.total} owned` : `${row.owned} titles`}</Meta>
      </Box>
    </Box>
  </Box>
);

const UsedBy = ({ usedBy }: { usedBy?: string[] }) =>
  usedBy?.length ? (
    <Box gap="inline">
      <SectionLabel>Used by</SectionLabel>
      <Box flexDirection="row" flexWrap="wrap" gap="inline">
        {usedBy.map((u) => (
          <Badge key={u} tone="info">{`used by ${u}`}</Badge>
        ))}
      </Box>
    </Box>
  ) : null;

const CollectionView = ({ row, onEdit }: { row: CollectionRow; onEdit: () => void }) => {
  const [copied, setCopied] = useState(false);
  if (row.kind === "TMDB")
    return (
      <Box gap="section">
        <CollectionHeader row={row} />
        <Panel gap="control">
          <ProgressTrack
            label={`${row.owned} of ${row.total} owned`}
            percent={Math.round((row.owned / (row.total ?? 1)) * 100)}
            tone="lock"
          />
          <Text textRole="body">{`${row.owned} of ${row.total} owned`}</Text>
          <Meta>
            TMDB keeps this collection up to date. It can't be edited here — copy it to make it yours.
          </Meta>
        </Panel>
        <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline">
          <Action onPress={() => setCopied(true)}>Copy to my collections</Action>
          {copied ? <Meta tone="success">✓ Copied as a Loomarr collection</Meta> : null}
        </Box>
      </Box>
    );
  const members = row.id === COLLECTION_EDIT.id ? COLLECTION_EDIT.members : [];
  return (
    <Box gap="section">
      <CollectionHeader row={row} />
      <UsedBy usedBy={row.usedBy} />
      {members.length ? (
        <Box gap="inline">
          <SectionLabel>Members, in order</SectionLabel>
          <Panel>
            {members.map((m, i) => (
              <Box alignItems="center" flexDirection="row" gap="control" key={m.title}>
                <Meta>{String(i + 1)}</Meta>
                <Art hue={m.hue} title={m.title} width={24} />
                <Text textRole="body">{m.title}</Text>
                <Meta>{String(m.year)}</Meta>
              </Box>
            ))}
          </Panel>
        </Box>
      ) : null}
      <Box flexDirection="row">
        <Action onPress={onEdit}>Edit collection</Action>
      </Box>
    </Box>
  );
};

const CollectionEditor = ({ row, onDone }: { row: CollectionRow; onDone: () => void }) => {
  const seeded = row.id === COLLECTION_EDIT.id;
  const [name, setName] = useState(seeded ? COLLECTION_EDIT.name : row.name);
  const [description, setDescription] = useState(seeded ? COLLECTION_EDIT.description : "");
  const [members, setMembers] = useState(seeded ? COLLECTION_EDIT.members : []);
  const [query, setQuery] = useState("");
  const usedBy = row.usedBy ?? [];
  const locked = usedBy.length > 0;

  return (
    <Box gap="section">
      <Box gap="inline">
        <SectionLabel>Editing collection</SectionLabel>
        <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline">
          <Badge tone="neutral">Loomarr · editable</Badge>
          {row.fromEmby ? <Badge tone="neutral">from Emby</Badge> : null}
          <UsedBadges usedBy={usedBy} />
        </Box>
      </Box>
      <Field label="Name" onChangeText={setName} value={name} />
      <Field label="Description" onChangeText={setDescription} value={description} />

      <Box gap="inline">
        <SectionLabel>{`Members · ${members.length}, in order`}</SectionLabel>
        <Panel>
          {members.length === 0 ? <Meta>No members yet — add a title below.</Meta> : null}
          {members.map((m, i) => (
            <Box alignItems="center" flexDirection="row" gap="control" key={m.title}>
              <Text accessibilityLabel={`Drag ${m.title} to reorder`} textRole="label" tone="muted">
                ⋮⋮
              </Text>
              <Meta>{String(i + 1)}</Meta>
              <Art hue={m.hue} title={m.title} width={24} />
              <Box flex={1} flexDirection="row" gap="inline" minWidth={0}>
                <Text numberOfLines={1} textRole="body">
                  {m.title}
                </Text>
                <Meta>{String(m.year)}</Meta>
              </Box>
              <Action
                onPress={() => setMembers((ms) => ms.filter((x) => x.title !== m.title))}
                variant="ghost"
              >
                Remove
              </Action>
            </Box>
          ))}
        </Panel>
        <Field
          accessibilityLabel="Add a member"
          onChangeText={setQuery}
          placeholder="Add a title — search your library"
          value={query}
        />
      </Box>

      <Box gap="inline">
        <Box flexDirection="row" flexWrap="wrap" gap="inline">
          <Action onPress={onDone}>Save</Action>
          <Action onPress={onDone} variant="outline">
            Cancel
          </Action>
          <Action disabled={locked} variant="destructive">
            Delete collection
          </Action>
        </Box>
        <Meta tone={locked ? "warning" : "muted"}>
          {locked
            ? `Can't delete while ${usedBy.join(" and ")} uses it. Take it off the channel first.`
            : "Not used by any channel, so it can be deleted."}
        </Meta>
      </Box>
    </Box>
  );
};

const UsedBadges = ({ usedBy }: { usedBy: string[] }) => (
  <>
    {usedBy.map((u) => (
      <Badge key={u} tone="info">{`used by ${u}`}</Badge>
    ))}
  </>
);

export { CollectionEditor, CollectionView };
