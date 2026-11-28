import { Action, Card, CompactSelect, Grid, Stack, Text, Tooltip } from "@loomarr/design-system";
import { SearchField } from "@loomarr/design-system/fields";
import { type ReactNode, useState } from "react";
import { AUDIENCE_LABEL, ROLE_LABEL } from "@/components/loomarr/filler/clip-card";
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

// On a phone the filters wrap onto their own full-width two-column row, last in the toolbar; on a
// wider screen they sit inline with the search and the view toggle.
const FilterGroup = ({ children, phone }: { children: ReactNode; phone: boolean }) =>
  phone ? (
    <Grid
      align="end"
      columnGap="3"
      columns={2}
      id="clip-filters"
      rowGap="3"
      style={{ flexBasis: "100%", order: 1 }}
    >
      {children}
    </Grid>
  ) : (
    <>{children}</>
  );

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
    <Card>
      <Stack
        align={phone ? "center" : "end"}
        direction="row"
        gap={phone ? "2" : "3"}
        paddingHorizontal={phone ? 12 : 16}
        paddingVertical={phone ? 10 : 16}
        wrap
      >
        {/* ⚠ Capped. `flex-1` alone stretched a clip-name box to ~900px on a 1440
          viewport — a text field far wider than anything typed into it, which reads
          as a layout bug rather than a generous input. It still grows on narrow
          screens (min-w-48) and stops being silly on wide ones. */}
        <Stack flex={1} maxWidth={phone ? undefined : 448} minWidth={phone ? 0 : 192}>
          <SearchField
            label="Search"
            labelHidden={phone}
            onChangeText={(text) => onFilterChange({ q: text || undefined })}
            placeholder="Clip name"
            value={q}
          />
        </Stack>
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
          <FilterGroup phone={phone}>
            <div>
              <Text as="label" htmlFor="clip-kind" textRole="label">
                Kind
              </Text>
              <CompactSelect
                id="clip-kind"
                label="Kind"
                onValueChange={(v) => onFilterChange({ role: v === ANY ? undefined : v })}
                options={KIND_OPTIONS}
                value={role || ANY}
              />
            </div>
            <div>
              <Text as="label" htmlFor="clip-audience" textRole="label">
                Audience
              </Text>
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
          </FilterGroup>
        ) : null}
      </Stack>
    </Card>
  );
};

export { CatalogToolbar };
