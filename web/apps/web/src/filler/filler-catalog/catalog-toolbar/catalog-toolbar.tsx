import { Action, Card, CompactSelect, Tooltip } from "@loomarr/design-system";
import { SearchField } from "@loomarr/ui";
import { useState } from "react";
import { AUDIENCE_LABEL, ROLE_LABEL } from "@/components/loomarr/filler/clip-card";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import type { FillerSearch } from "../../filler-search";
import { type CatalogView, CatalogViewToggle } from "../catalog-view-toggle";

// The no-filter choice; its URL form is an absent filter.
const ANY = "any";
const KIND_OPTIONS = [
  { label: "Any", value: ANY },
  ...Object.entries(ROLE_LABEL).map(([value, label]) => ({ label, value })),
];
const AUDIENCE_OPTIONS = [
  { label: "Any", value: ANY },
  ...Object.entries(AUDIENCE_LABEL).map(([value, label]) => ({ label, value })),
];

interface CatalogToolbarProps {
  /** A phone gets the folded toolbar: search, Filters and the view toggle on one line. */
  phone: boolean;
  q: string;
  role: string;
  audience: string;
  untagged: boolean;
  view: string;
  /** Merges a partial change into the URL filters (and resets the page). */
  onFilterChange: (next: Partial<FillerSearch>) => unknown;
  onViewChange: (view: CatalogView) => unknown;
}

// The catalog's filter bar: name search, role, audience, "Untagged only", and the grid/list toggle.
// On a phone the toolbar folds to one line — search, Filters, and the view toggle — so the first
// clips land above the fold (#1817, decided on #1908). Filters opens the rest.
const CatalogToolbar = ({
  phone,
  q,
  role,
  audience,
  untagged,
  view,
  onFilterChange,
  onViewChange,
}: CatalogToolbarProps) => {
  const [filtersOpen, setFiltersOpen] = useState(false);

  return (
    <Card
      alignItems={phone ? "center" : "flex-end"}
      flexDirection="row"
      flexWrap="wrap"
      gap={phone ? 8 : 12}
      paddingHorizontal={phone ? 12 : 16}
      paddingVertical={phone ? 10 : 16}
    >
      {/* ⚠ Capped. `flex-1` alone stretched a clip-name box to ~900px on a 1440
          viewport — a text field far wider than anything typed into it, which reads
          as a layout bug rather than a generous input. It still grows on narrow
          screens (min-w-48) and stops being silly on wide ones. */}
      <div className={cn("min-w-0 flex-1", !phone && "max-w-md min-w-48")}>
        <SearchField
          label="Search"
          labelHidden={phone}
          onChangeText={(text) => onFilterChange({ q: text || undefined })}
          placeholder="Clip name"
          value={q}
        />
      </div>
      {phone ? (
        <Action
          aria-controls="clip-filters"
          aria-expanded={filtersOpen}
          onPress={() => setFiltersOpen((open) => !open)}
          variant="outline"
        >
          Filters
        </Action>
      ) : null}
      <CatalogViewToggle view={view} phone={phone} onChange={onViewChange} />
      {!phone || filtersOpen ? (
        <div
          id="clip-filters"
          className={cn(
            "flex items-end gap-3",
            phone ? "order-last grid basis-full grid-cols-2" : "contents",
          )}
        >
          <div>
            <Label htmlFor="clip-kind">Kind</Label>
            <CompactSelect
              id="clip-kind"
              label="Kind"
              onValueChange={(v) => onFilterChange({ role: v === ANY ? undefined : v })}
              options={KIND_OPTIONS}
              value={role || ANY}
            />
          </div>
          <div>
            <Label htmlFor="clip-audience">Audience</Label>
            <CompactSelect
              id="clip-audience"
              label="Audience"
              onValueChange={(v) => onFilterChange({ audience: v === ANY ? undefined : v })}
              options={AUDIENCE_OPTIONS}
              value={audience || ANY}
            />
          </div>
          {/* "Untagged" means a COMMERCIAL missing a match tag — bumpers do their job
              without era/audience, so they are never counted as needing work (§10). */}
          <Tooltip content="Commercials missing era, audience, or category">
            <Action
              aria-pressed={untagged}
              onPress={() => onFilterChange({ untagged: untagged ? undefined : true })}
              selected={untagged}
              tone="secondary"
            >
              Untagged only
            </Action>
          </Tooltip>
        </div>
      ) : null}
    </Card>
  );
};

export { CatalogToolbar };
