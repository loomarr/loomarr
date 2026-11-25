import { Action, Surface, Text, useWindowWidth } from "@loomarr/design-system";
import { type ReactNode, useState } from "react";

import { LibraryPoster, Line, Stack } from "../library-poster";
import type {
  LibraryCollectionChannel,
  LibraryCollectionPoster,
  LibraryCollectionsProps,
} from "./library-collections.type";

// COLLECTIONS (#2151 decision 28, design/library variant D): the household's own collections under
// Yours, then the ready-made sets from TMDB. Each card is a pile of posters beside its name; the
// sets can't be changed, only copied. Plain words per the mock's word sheet.

// "On Saturday Morning (channel 4)"
const channelPhrase = (c: LibraryCollectionChannel) => `On ${c.name} (channel ${c.number})`;

// Up to three posters fanned behind one another, the front one leading; the ones behind are dimmer.
// Bare, as in the mock: the collection's name sits beside the pile, so no poster draws a title.
const PILE_WIDTH = 96;
const PosterPile = ({ pile }: { pile: readonly LibraryCollectionPoster[] }) => {
  const shown = pile.slice(0, 3);
  // Back to front, so the first title is drawn last and sits on top.
  const layers = [2, 1, 0].filter((i) => i === 0 || i < shown.length);
  return (
    <Stack aria-hidden flexShrink={0} height={160} position="relative" width={PILE_WIDTH + 16}>
      {layers.map((i) => (
        <Stack key={i} left={i * 8} position="absolute" top={(2 - i) * 6} width={PILE_WIDTH}>
          <LibraryPoster art={shown[i]?.poster} dim={i > 0} name="" />
        </Stack>
      ))}
    </Stack>
  );
};

const Card = ({
  name,
  pile,
  lines,
  action,
}: {
  name: string;
  pile: readonly LibraryCollectionPoster[];
  lines: readonly { text: string; tone: "secondary" | "muted" }[];
  /** Absent on a card the viewer can only read. */
  action?: ReactNode;
}) => (
  <Surface flexBasis={0} flexGrow={1} gap="control" minWidth={0} padding="control">
    <Line alignItems="flex-start" flexWrap="nowrap" gap="control">
      <PosterPile pile={pile} />
      <Stack flexGrow={1} flexShrink={1} gap={2} minWidth={0}>
        <Text as="h3" textRole="title">
          {name}
        </Text>
        {lines.map((l) => (
          <Text key={l.text} textRole="compact" tone={l.tone}>
            {l.text}
          </Text>
        ))}
      </Stack>
    </Line>
    {action ? <Line>{action}</Line> : null}
  </Surface>
);

// "Make my own copy", then "Copied — it's under Yours" once it is.
const CopyAction = ({ onCopy }: { onCopy: () => Promise<void> }) => {
  const [state, setState] = useState<"idle" | "copying" | "copied" | "failed">("idle");
  const copy = () => {
    setState("copying");
    onCopy().then(
      () => setState("copied"),
      () => setState("failed"),
    );
  };
  return (
    <Stack gap="inline">
      <Action disabled={state === "copying" || state === "copied"} onPress={copy} variant="outline">
        {state === "copied" ? "Copied — it's under Yours" : "Make my own copy"}
      </Action>
      {state === "failed" ? (
        <Text textRole="compact" tone="secondary">
          Couldn't make a copy just now. Try again in a moment.
        </Text>
      ) : null}
    </Stack>
  );
};

// Cards three across on a desktop, two on a tablet, one on a phone.
const Grid = ({ children }: { children: ReactNode[] }) => {
  const width = useWindowWidth();
  const columns = width >= 1024 ? 3 : width >= 768 ? 2 : 1;
  const rows: ReactNode[][] = [];
  for (let i = 0; i < children.length; i += columns) rows.push(children.slice(i, i + columns));
  const slots = Array.from({ length: columns }, (_, i) => `slot-${i}`);
  return (
    <Stack gap="control">
      {rows.map((row, r) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: a row is layout only, keyed by its place
        <Line alignItems="stretch" flexWrap="nowrap" gap="control" key={r}>
          {row}
          {slots.slice(row.length).map((slot) => (
            <Stack aria-hidden flexBasis={0} flexGrow={1} flexShrink={1} key={slot} />
          ))}
        </Line>
      ))}
    </Stack>
  );
};

// The household's own collections as cards; Edit on each only when the viewer can change them.
const CollectionCards = ({
  collections,
  onEdit,
}: {
  collections: LibraryCollectionsProps["collections"];
  onEdit?: (id: string) => void;
}) => (
  <Grid>
    {collections.map((c) => (
      <Card
        action={
          onEdit ? (
            <Action accessibilityLabel={`Edit ${c.name}`} onPress={() => onEdit(c.id)} variant="outline">
              Edit
            </Action>
          ) : null
        }
        key={c.id}
        lines={[
          { text: c.titles === 1 ? "1 title" : `${c.titles} titles`, tone: "secondary" },
          ...(c.importedFrom
            ? [{ text: `Brought over from ${c.importedFrom}`, tone: "muted" as const }]
            : []),
          ...c.usedBy.map((u) => ({ text: channelPhrase(u), tone: "muted" as const })),
        ]}
        name={c.name}
        pile={c.pile}
      />
    ))}
  </Grid>
);

// None made yet (#4476 mock): what a collection is, in place of an empty Yours. An admin can make
// one; a member is told where they come from.
const NoneYet = ({ onNew }: { onNew?: () => void }) => (
  <Stack aria-label="Yours" gap="control" role="region">
    <Text as="h2" textRole="headline">
      No collections yet
    </Text>
    <Text textRole="body" tone="secondary">
      {onNew
        ? "A collection is a set of titles you put together, like “Rainy Sunday films”. A channel can play from it."
        : "When someone in your household puts titles together, like “Rainy Sunday films”, they show up here."}
    </Text>
    {onNew ? (
      <Line>
        <Action icon="plus" onPress={onNew} tone="primary">
          New collection
        </Action>
      </Line>
    ) : null}
  </Stack>
);

const LibraryCollections = ({ collections, sets, manage }: LibraryCollectionsProps) => {
  const none = collections.length === 0;
  // A member reads the collections only: no New collection, no Edit, and no ready-made sets,
  // whose one action is to copy one.
  if (!manage) {
    return none ? <NoneYet /> : <CollectionCards collections={collections} />;
  }
  return (
    <Stack gap="section">
      {none ? (
        <NoneYet onNew={manage.onNew} />
      ) : (
        <Stack aria-label="Yours" gap="control" role="region">
          <Line justifyContent="space-between" rowGap="control">
            <Text as="h2" textRole="headline">
              Yours
            </Text>
            <Action icon="plus" onPress={manage.onNew} tone="primary">
              New collection
            </Action>
          </Line>
          <CollectionCards collections={collections} onEdit={manage.onEdit} />
        </Stack>
      )}
      {sets.length > 0 ? (
        <Stack aria-label="Ready-made sets" gap="control" role="region">
          <Text as="h2" textRole={none ? "label" : "headline"} tone={none ? "secondary" : undefined}>
            {none ? "Or start from a ready-made set" : "Ready-made sets"}
          </Text>
          <Grid>
            {sets.map((s) => (
              <Card
                action={<CopyAction onCopy={() => manage.onCopy(s.id)} />}
                key={s.id}
                lines={[{ text: `From TMDB — you have ${s.have} of ${s.total}`, tone: "secondary" }]}
                name={s.name}
                pile={s.pile}
              />
            ))}
          </Grid>
        </Stack>
      ) : null}
    </Stack>
  );
};

export { LibraryCollections };
