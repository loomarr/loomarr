import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { FillerSearch } from "../../filler-search";
import { type CatalogView, CatalogViewToggle } from "../catalog-view-toggle";

interface CatalogToolbarProps {
  /** A phone gets the folded toolbar: search, Filters and the view toggle on one line. */
  phone: boolean;
  q: string;
  kind: string;
  audience: string;
  untagged: boolean;
  view: string;
  /** Merges a partial change into the URL filters (and resets the page). */
  onFilterChange: (next: Partial<FillerSearch>) => unknown;
  onViewChange: (view: CatalogView) => unknown;
}

// The catalog's filter bar: name search, kind, audience, "Untagged only", and the grid/list toggle.
// On a phone the toolbar folds to one line — search, Filters, and the view toggle — so the first
// clips land above the fold (#1817, decided on #1908). Filters opens the rest.
const CatalogToolbar = ({
  phone,
  q,
  kind,
  audience,
  untagged,
  view,
  onFilterChange,
  onViewChange,
}: CatalogToolbarProps) => {
  const [filtersOpen, setFiltersOpen] = useState(false);

  return (
    <Card className={cn("flex flex-wrap", phone ? "items-center gap-2 px-3 py-2.5" : "items-end gap-3 p-4")}>
      {/* ⚠ Capped. `flex-1` alone stretched a clip-name box to ~900px on a 1440
          viewport — a text field far wider than anything typed into it, which reads
          as a layout bug rather than a generous input. It still grows on narrow
          screens (min-w-48) and stops being silly on wide ones. */}
      <div className={cn("min-w-0 flex-1", !phone && "max-w-md min-w-48")}>
        <Label htmlFor="clip-search" className={cn(phone && "sr-only")}>
          Search
        </Label>
        <Input
          id="clip-search"
          value={q}
          placeholder="Clip name"
          onChange={(e) => onFilterChange({ q: e.target.value || undefined })}
        />
      </div>
      {phone ? (
        <Button
          variant="outline"
          size="sm"
          aria-expanded={filtersOpen}
          aria-controls="clip-filters"
          onClick={() => setFiltersOpen((open) => !open)}
        >
          Filters
        </Button>
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
            {/* "any" sentinel ↔ "" (Radix forbids an empty value) — the no-filter default. */}
            <Select
              value={kind || "any"}
              onValueChange={(v) => onFilterChange({ kind: v === "any" ? undefined : v })}
            >
              <SelectTrigger id="clip-kind">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="any">Any</SelectItem>
                <SelectItem value="unclassified">Unclassified</SelectItem>
                <SelectItem value="commercial">Commercial</SelectItem>
                <SelectItem value="bumper">Bumper</SelectItem>
                <SelectItem value="station_id">Station ID</SelectItem>
                <SelectItem value="psa">PSA</SelectItem>
                <SelectItem value="trailer">Trailer</SelectItem>
                <SelectItem value="interstitial">Interstitial</SelectItem>
                <SelectItem value="political">Political ad</SelectItem>
                <SelectItem value="news">News &amp; weather</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div>
            <Label htmlFor="clip-audience">Audience</Label>
            <Select
              value={audience || "any"}
              onValueChange={(v) => onFilterChange({ audience: v === "any" ? undefined : v })}
            >
              <SelectTrigger id="clip-audience">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="any">Any</SelectItem>
                <SelectItem value="kids">Kids</SelectItem>
                <SelectItem value="family">Family</SelectItem>
                <SelectItem value="general">General</SelectItem>
                <SelectItem value="late_night">Late night</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Button
            variant={untagged ? "default" : "outline"}
            size="sm"
            onClick={() => onFilterChange({ untagged: untagged ? undefined : true })}
            // "Untagged" means a COMMERCIAL missing a match tag — bumpers do their job
            // without era/audience, so they are never counted as needing work (§10).
            title="Commercials missing era, audience, or category"
          >
            Untagged only
          </Button>
        </div>
      ) : null}
    </Card>
  );
};

export { CatalogToolbar };
