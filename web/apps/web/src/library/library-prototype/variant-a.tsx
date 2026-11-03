// PROTOTYPE — throwaway (#2151 step 2). Variant A — "Table + side sheet": a full-width dense Items
// table, the item view and the collection editor in a right side sheet over it, Unmatched as a
// stack of review cards.
import { Tabs } from "@loomarr/design-system";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { FARGO, type ItemRow, itemsFor, scanStatus, WEST_WING } from "./prototype-data";
import type { Screen, VariantProps } from "./prototype-state";
import { ScanHealth } from "./scan-health";
import { CollectionEditor, CollectionsTable } from "./variant-a-collections";
import { ItemView } from "./variant-a-item-view";
import { ItemsTable } from "./variant-a-items";
import { SideSheet } from "./variant-a-parts";
import { UnmatchedStack } from "./variant-a-unmatched";

type Tab = "items" | "unmatched" | "collections";

const tabFor = (screen: Screen): Tab =>
  screen === "unmatched"
    ? "unmatched"
    : screen === "collections" || screen === "collection-edit"
      ? "collections"
      : "items";

const VariantA = ({ screen, scan, device, go }: VariantProps) => {
  const phone = device === "phone";
  const status = scanStatus(scan);
  const items = itemsFor(scan);
  const tab = tabFor(screen);

  const openItem = (row: ItemRow) => go(row.kind === "Movie" ? "item-movie" : "item-series");
  const selectedId =
    screen === "item-movie"
      ? FARGO.id
      : screen === "item-series" || screen === "provenance"
        ? WEST_WING.id
        : undefined;
  const detail = screen === "item-movie" ? FARGO : WEST_WING;
  const showItemSheet = screen === "item-series" || screen === "item-movie" || screen === "provenance";

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader title="Library" description="What Loomarr knows about the media files in your household." />
      <div className="flex flex-col gap-4 px-4 py-4 sm:px-6">
        <ScanHealth density="compact" onOpenSettings={() => go("settings")} status={status} />
        <div className="overflow-x-auto">
          <Tabs
            label="Library sections"
            onValueChange={(next: Tab) => go(next)}
            options={[
              { label: "Items", value: "items" },
              { label: `Unmatched (${status.unmatched})`, value: "unmatched" },
              { label: "Collections", value: "collections" },
            ]}
            value={tab}
          />
        </div>

        {tab === "items" ? (
          <ItemsTable
            fresh={scan === "fresh"}
            items={items}
            onOpen={openItem}
            onOpenSettings={() => go("settings")}
            phone={phone}
            selectedId={selectedId}
          />
        ) : null}
        {tab === "unmatched" ? <UnmatchedStack count={status.unmatched} /> : null}
        {tab === "collections" ? <CollectionsTable onEdit={() => go("collection-edit")} /> : null}
      </div>

      {showItemSheet ? (
        <SideSheet
          onClose={() => go("items")}
          phone={phone}
          subtitle={`${detail.kind} · ${detail.year}`}
          title={detail.title}
        >
          <ItemView item={detail} key={screen} provenanceOpen={screen === "provenance"} />
        </SideSheet>
      ) : null}
      {screen === "collection-edit" ? (
        <SideSheet
          onClose={() => go("collections")}
          phone={phone}
          subtitle="Your collection"
          title="Edit collection"
        >
          <CollectionEditor onDone={() => go("collections")} />
        </SideSheet>
      ) : null}
    </div>
  );
};

export { VariantA };
