import { Action, Field, Surface, Text, useWindowWidth } from "@loomarr/design-system";
import { type ReactNode, useEffect, useState } from "react";

import type { LibraryCollectionChannel } from "../library-collections/library-collections.type";
import { LibraryPoster, Line, PAGE_GUTTER, Stack } from "../library-poster";
import type {
  LibraryCollectionEditorProps,
  LibraryCollectionMember,
  LibraryCollectionSortable,
} from "./library-collection-editor.type";

// THE COLLECTION EDITOR (#2151 decision 28, design/library variant D): its name and description,
// its titles in order (dragged into a new one), "Add a title" from the library, and Delete, which
// waits while a channel uses it. Presentational: the app owns the requests and the drag and drop.

const SEARCH_SETTLE_MS = 300;

const channelPhrase = (c: LibraryCollectionChannel) => `On ${c.name} (channel ${c.number})`;

// The search as typed, settled for a moment before it is asked.
const useSettled = (value: string) => {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setSettled(value), SEARCH_SETTLE_MS);
    return () => clearTimeout(t);
  }, [value]);
  return settled;
};

const Thumb = ({ member }: { member: LibraryCollectionMember }) => (
  <Stack flexShrink={0} width={36}>
    <LibraryPoster art={member.poster} name={member.poster ? member.name : ""} />
  </Stack>
);

const NameYear = ({ member }: { member: LibraryCollectionMember }) => (
  <Stack flexBasis={0} flexGrow={1} minWidth={0}>
    <Text textRole="body">{member.name}</Text>
    {member.year ? (
      <Text textRole="compact" tone="secondary">
        {String(member.year)}
      </Text>
    ) : null}
  </Stack>
);

// No drag and drop: the rows keep their order.
const STILL: LibraryCollectionSortable = {
  List: ({ children }) => <>{children}</>,
  Row: ({ children }) => <>{children}</>,
  Grip: ({ children }) => <>{children}</>,
};

const Members = ({
  members,
  onMove,
  onRemove,
  sortable = STILL,
}: Pick<LibraryCollectionEditorProps, "members" | "onMove" | "onRemove" | "sortable">) => {
  const { List, Row, Grip } = sortable;
  return (
    <Stack gap={4}>
      <List ids={members.map((m) => m.id)} onMove={onMove}>
        {members.map((m) => (
          <Row id={m.id} key={m.id}>
            <Line
              borderBottomColor="borderDecorative"
              borderBottomWidth={1}
              borderRadius={0}
              flexWrap="nowrap"
              paddingVertical="inline"
            >
              <Grip label={`Move ${m.name}`}>
                <Text textRole="body" tone="muted">
                  ⠿
                </Text>
              </Grip>
              <Thumb member={m} />
              <NameYear member={m} />
              <Action accessibilityLabel={`Remove ${m.name}`} onPress={() => onRemove(m.id)} variant="ghost">
                Remove
              </Action>
            </Line>
          </Row>
        ))}
      </List>
    </Stack>
  );
};

type Found = { query: string; results?: readonly LibraryCollectionMember[]; failed?: boolean };

// "Add a title": what a search of the library finds, each with Add, as "Pick a different title" does.
const AddTitle = ({
  members,
  onSearch,
  onAdd,
}: Pick<LibraryCollectionEditorProps, "members" | "onSearch" | "onAdd">) => {
  const [typed, setTyped] = useState("");
  const query = useSettled(typed.trim());
  const [found, setFound] = useState<Found>({ query: "" });
  useEffect(() => {
    if (query === "") return;
    let current = true;
    onSearch(query).then(
      (results) => current && setFound({ query, results }),
      () => current && setFound({ query, failed: true }),
    );
    return () => {
      current = false;
    };
  }, [query, onSearch]);
  const searched = query !== "" && found.query === query;
  const inIt = new Set(members.map((m) => m.id));
  const list = searched ? (found.results ?? []).filter((r) => !inIt.has(r.id)) : [];
  return (
    <Stack gap="inline">
      <Field label="Add a title" onChangeText={setTyped} placeholder="Search your library" value={typed} />
      {searched && found.failed ? (
        <Text textRole="body" tone="secondary">
          Couldn't search just now. Try again in a moment.
        </Text>
      ) : searched && list.length === 0 ? (
        <Text textRole="body" tone="secondary">{`Nothing matches "${query}".`}</Text>
      ) : null}
      {list.map((r) => (
        <Line flexWrap="nowrap" key={r.id}>
          <Thumb member={r} />
          <NameYear member={r} />
          <Action accessibilityLabel={`Add ${r.name}`} onPress={() => onAdd(r)} variant="secondary">
            Add
          </Action>
        </Line>
      ))}
    </Stack>
  );
};

const Footer = ({
  saved,
  usedBy,
  busy,
  failed,
  name,
  onDelete,
  onCancel,
  onSave,
}: Pick<
  LibraryCollectionEditorProps,
  "saved" | "usedBy" | "busy" | "failed" | "name" | "onDelete" | "onCancel" | "onSave"
>): ReactNode => {
  const used = usedBy.length > 0;
  return (
    <Stack gap="inline">
      <Line alignItems="flex-start" justifyContent="space-between" rowGap="control">
        {saved ? (
          <Stack gap={4}>
            <Line>
              <Action disabled={used || busy} onPress={onDelete} tone="danger" variant="outline">
                Delete collection
              </Action>
            </Line>
            <Text textRole="compact" tone="muted">
              {used
                ? `${usedBy.map(channelPhrase).join(" and ")} uses this. Take it off the channel first.`
                : "No channel uses this collection."}
            </Text>
          </Stack>
        ) : (
          <Stack />
        )}
        <Line>
          <Action disabled={busy} onPress={onCancel} variant="ghost">
            Cancel
          </Action>
          <Action disabled={busy || name.trim() === ""} onPress={onSave} tone="primary">
            Save
          </Action>
        </Line>
      </Line>
      {failed ? (
        <Text textRole="body" tone="secondary">
          {failed === "save"
            ? "Couldn't save just now. Try again in a moment."
            : "Couldn't delete it just now. Try again in a moment."}
        </Text>
      ) : null}
    </Stack>
  );
};

const LibraryCollectionEditor = (props: LibraryCollectionEditorProps) => {
  const wide = useWindowWidth() >= 768;
  return (
    <Stack
      alignSelf="center"
      gap="section"
      maxWidth={768 + 2 * PAGE_GUTTER.wide}
      padding={wide ? PAGE_GUTTER.wide : PAGE_GUTTER.phone}
      width="100%"
    >
      <Line>
        <Action icon="back" onPress={props.onBack} variant="ghost">
          Collections
        </Action>
      </Line>
      <Text as="h1" textRole="display">
        {props.name.trim() || "New collection"}
      </Text>
      <Surface gap="control" padding="section">
        <Field label="Name" onChangeText={props.onNameChange} value={props.name} />
        <Field label="Description" onChangeText={props.onDescriptionChange} value={props.description} />
      </Surface>
      <Surface gap="control" padding="section">
        <Text as="h2" textRole="headline">
          In this collection
        </Text>
        <Text textRole="compact" tone="secondary">
          Drag to change the order.
        </Text>
        <Members
          members={props.members}
          onMove={props.onMove}
          onRemove={props.onRemove}
          sortable={props.sortable}
        />
        <AddTitle members={props.members} onAdd={props.onAdd} onSearch={props.onSearch} />
      </Surface>
      <Footer
        busy={props.busy}
        failed={props.failed}
        name={props.name}
        onCancel={props.onCancel}
        onDelete={props.onDelete}
        onSave={props.onSave}
        saved={props.saved}
        usedBy={props.usedBy}
      />
    </Stack>
  );
};

export { LibraryCollectionEditor };
