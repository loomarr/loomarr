// PROTOTYPE — throwaway (#2151 step 2). Variant B — "Split view": a persistent master–detail page.
// LEFT (~40%): title, tabs, compact scan-health line, a compact list. RIGHT: the selected thing's
// detail, always visible. Under 900px it collapses to list → detail with a back affordance.
import {
  Action,
  AdaptiveSplit,
  resolveAdaptiveDirection,
  Surface,
  Tabs,
  Text,
  useWindowWidth,
} from "@loomarr/design-system";
import { useState } from "react";
import {
  COLLECTIONS,
  FARGO,
  type ItemRow,
  itemsFor,
  scanStatus,
  UNMATCHED,
  WEST_WING,
} from "./prototype-data";
import type { Screen, VariantProps } from "./prototype-state";
import { ScanHealth } from "./scan-health";
import { CollectionEditor, CollectionView } from "./variant-b-collection";
import { ItemStub, ItemView } from "./variant-b-item";
import { CollectionsList, ItemsList, UnmatchedList } from "./variant-b-lists";
import { Box, EmptyNote } from "./variant-b-parts";
import { ReviewCard } from "./variant-b-review";

type Tab = "items" | "unmatched" | "collections";

const tabOf = (screen: Screen): Tab =>
  screen === "unmatched"
    ? "unmatched"
    : screen === "collections" || screen === "collection-edit"
      ? "collections"
      : "items";

// What a screen means for the page's local selection.
const seed = (screen: Screen, items: ItemRow[]) => ({
  itemId:
    screen === "item-movie"
      ? "fargo"
      : screen === "item-series" || screen === "provenance"
        ? "west-wing"
        : (items[0]?.id ?? null),
  groupId: UNMATCHED[0]?.id ?? null,
  collectionId: screen === "collection-edit" ? "hitchcock" : (COLLECTIONS[0]?.id ?? null),
  editing: screen === "collection-edit",
  openField: screen === "provenance" ? "Certification" : null,
  // phone: item views and the editor open on the detail; the three lists open on the list
  detail:
    screen === "item-series" ||
    screen === "item-movie" ||
    screen === "provenance" ||
    screen === "collection-edit",
});

const VariantB = ({ screen, scan, go }: VariantProps) => {
  const status = scanStatus(scan);
  const items = itemsFor(scan);
  const [seededFor, setSeededFor] = useState(`${screen}|${scan}`);
  const [sel, setSel] = useState(() => seed(screen, items));
  const [decided, setDecided] = useState<Record<string, string>>({});
  const width = useWindowWidth();
  const wide = resolveAdaptiveDirection(width, "pointer") === "row";

  // A new screen or scan state from the switcher re-seeds the selection (state adjusted in render).
  if (seededFor !== `${screen}|${scan}`) {
    setSeededFor(`${screen}|${scan}`);
    setSel(seed(screen, items));
  }

  const tab = tabOf(screen);
  const patch = (next: Partial<typeof sel>) => setSel((s) => ({ ...s, ...next }));
  const pick = (next: Partial<typeof sel>) => patch({ ...next, detail: true });

  const unmatchedCount = status.unmatched;
  const header = (
    <Box gap="control">
      <Text textRole="title">Library</Text>
      <Tabs
        label="Library sections"
        onValueChange={(v) => go(v)}
        options={[
          { value: "items", label: "Items" },
          { value: "unmatched", label: `Unmatched (${unmatchedCount})` },
          { value: "collections", label: "Collections" },
        ]}
        value={tab}
      />
      <ScanHealth density="compact" onOpenSettings={() => go("settings")} status={status} />
    </Box>
  );

  const list =
    tab === "items" ? (
      <ItemsList
        items={items}
        onOpenSettings={() => go("settings")}
        onSelect={(id) => pick({ itemId: id, openField: null })}
        selectedId={sel.itemId}
      />
    ) : tab === "unmatched" ? (
      <UnmatchedList
        decided={decided}
        groups={scan === "fresh" ? [] : UNMATCHED}
        onSelect={(id) => pick({ groupId: id })}
        selectedId={sel.groupId}
      />
    ) : (
      <CollectionsList
        onCopy={() => pick({ collectionId: "alien", editing: false })}
        onNew={() => pick({ collectionId: "hitchcock", editing: true })}
        onSelect={(id) => pick({ collectionId: id, editing: false })}
        rows={COLLECTIONS}
        selectedId={sel.collectionId}
      />
    );

  const detail = (() => {
    if (tab === "items") {
      const row = items.find((i) => i.id === sel.itemId);
      if (!row)
        return (
          <EmptyNote
            body="Select an item from the list to see where each value came from."
            title="No item selected"
          />
        );
      const offline = Boolean(row.offline);
      const full = row.id === WEST_WING.id ? WEST_WING : row.id === FARGO.id ? FARGO : null;
      return full ? (
        <ItemView
          detail={full}
          key={`${row.id}|${scan}`}
          offline={offline}
          onToggleField={(label) => patch({ openField: sel.openField === label ? null : label })}
          openField={sel.openField}
          row={row}
        />
      ) : (
        <ItemStub offline={offline} row={row} />
      );
    }
    if (tab === "unmatched") {
      const group = scan === "fresh" ? undefined : UNMATCHED.find((g) => g.id === sel.groupId);
      if (!group)
        return (
          <EmptyNote
            body="Files Loomarr can't ground in an identity show up here for review."
            title="Nothing to review"
          />
        );
      return (
        <ReviewCard
          decision={decided[group.id]}
          group={group}
          key={group.id}
          onDecide={(label) =>
            setDecided((d) => {
              const next = { ...d };
              if (label) next[group.id] = label;
              else delete next[group.id];
              return next;
            })
          }
        />
      );
    }
    const row = COLLECTIONS.find((c) => c.id === sel.collectionId);
    if (!row)
      return <EmptyNote body="Pick a collection to see its members." title="No collection selected" />;
    return sel.editing && row.kind === "Mine" ? (
      <CollectionEditor key={row.id} onDone={() => patch({ editing: false })} row={row} />
    ) : (
      <CollectionView key={row.id} onEdit={() => patch({ editing: true })} row={row} />
    );
  })();

  const pane = (children: React.ReactNode) => (
    <div
      className={wide ? "min-h-0 overflow-y-auto px-1 pb-6" : undefined}
      style={wide ? { maxHeight: "calc(100dvh - 4rem)" } : undefined}
    >
      {children}
    </div>
  );

  if (wide)
    return (
      <div className="p-4">
        <AdaptiveSplit
          gap="control"
          primary={pane(
            <Box gap="control">
              {header}
              {list}
            </Box>,
          )}
          secondary={pane(
            <Surface level="raised" padding="section">
              {detail}
            </Surface>,
          )}
          secondaryWidth="60%"
        />
      </div>
    );

  return (
    <div className="p-4 pb-24">
      {sel.detail ? (
        <Box gap="control">
          <Box alignItems="flex-start">
            <Action icon="back" onPress={() => patch({ detail: false })} variant="ghost">
              {tab === "items" ? "Items" : tab === "unmatched" ? "Unmatched" : "Collections"}
            </Action>
          </Box>
          {detail}
        </Box>
      ) : (
        <Box gap="control">
          {header}
          {list}
        </Box>
      )}
    </div>
  );
};

export { VariantB };
