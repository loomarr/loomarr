// PROTOTYPE — throwaway (#2151 step 2). Variant C — "Pages + review mode": every view is its own
// full page. Items is a table page, an item is a separate page with section tabs, Unmatched is a
// focused one-at-a-time review mode, Collections is a card grid with a full-page editor.
import { Tabs, Text } from "@loomarr/design-system";
import { scanStatus } from "./prototype-data";
import type { Screen, VariantProps } from "./prototype-state";
import { ScanHealth } from "./scan-health";
import { CollectionEditPage, CollectionsPage } from "./variant-c-collections";
import { ItemPage } from "./variant-c-item";
import { ItemsPage } from "./variant-c-items";
import { ReviewPage } from "./variant-c-review";

type LibraryTab = "items" | "unmatched" | "collections";

const tabOf = (screen: Screen): LibraryTab =>
  screen === "unmatched"
    ? "unmatched"
    : screen === "collections" || screen === "collection-edit"
      ? "collections"
      : "items";

const VariantC = ({ screen, scan, go }: VariantProps) => {
  const status = scanStatus(scan);
  const isItemPage = screen === "item-series" || screen === "item-movie" || screen === "provenance";
  const isEditPage = screen === "collection-edit";
  // Detail pages are their own page: no Library header or tabs, just the breadcrumb.
  const chrome = !isItemPage && !isEditPage;

  return (
    <div className="flex flex-col gap-4 p-4 md:p-6">
      {chrome ? (
        <>
          <div className="flex flex-col gap-3">
            <Text textRole="title">Library</Text>
            <Tabs
              label="Library sections"
              onValueChange={(t) => go(t)}
              options={[
                { label: "Items", value: "items" },
                { label: `Unmatched (${status.unmatched})`, value: "unmatched" },
                { label: "Collections", value: "collections" },
              ]}
              value={tabOf(screen)}
            />
          </div>
          <ScanHealth density="compact" onOpenSettings={() => go("settings")} status={status} />
        </>
      ) : null}

      {screen === "items" ? <ItemsPage go={go} scan={scan} /> : null}
      {isItemPage ? <ItemPage go={go} key={screen} screen={screen} /> : null}
      {screen === "unmatched" ? <ReviewPage /> : null}
      {screen === "collections" ? <CollectionsPage go={go} /> : null}
      {isEditPage ? <CollectionEditPage go={go} /> : null}
    </div>
  );
};

export { VariantC };
