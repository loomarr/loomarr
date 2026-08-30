import * as fillerApi from "@loomarr/api/endpoints/filler";
import { ClipDTOAudience } from "@loomarr/api/models/clipDTOAudience";
import { ClipDTOKind } from "@loomarr/api/models/clipDTOKind";
import type { FillerSelection } from "@loomarr/api/models/fillerSelection";
import { useState } from "react";
import { FieldHelp } from "@/components/loomarr/feedback/field-help";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";

// FieldLabel — a label + (i) help icon, replacing the permanent helper `<p>` under each
// control (mirrors ChannelPolicyFields). Help on hover keeps the criteria form compact.
const FieldLabel = ({ htmlFor, children, help }: { htmlFor?: string; children: string; help: string }) => (
  <div className="flex items-center gap-1.5">
    <Label htmlFor={htmlFor}>{children}</Label>
    <FieldHelp label={children}>{help}</FieldHelp>
  </div>
);

// ⚠ The hardcoded category mirror is GONE (§10 V45a). Categories are now the PRODUCT-axis taxa of the
// operator-editable taxonomy graph, fetched live from /v1/taxonomy — the one source of truth, so this
// selector cannot drift from what the backend accepts. Selecting a ROLLUP node (e.g. `food`) now
// matches every descendant (`cereal`, `candy`…) via the server's rollup-set intersection. The label
// is a breadcrumb rather than a flattened leaf, so "Food › Cereal" explains why broad and specific
// choices both exist.
const useProductCategories = (): { slug: string; label: string }[] => {
  const vocab = fillerApi.useListTaxonomy();
  // The orval hook wraps the body: data.status===200 ? data.data.<body>. Match the page's own pattern.
  const taxa = vocab.data?.status === 200 ? (vocab.data.data.taxa ?? []) : [];
  const products = taxa.filter((t) => t.axis === "product");
  const bySlug = new Map(products.map((taxon) => [taxon.slug, taxon]));
  const breadcrumb = (slug: string) => {
    const labels: string[] = [];
    const seen = new Set<string>();
    let current = bySlug.get(slug);
    while (current && !seen.has(current.slug)) {
      seen.add(current.slug);
      labels.unshift(current.label);
      current = current.parent ? bySlug.get(current.parent) : undefined;
    }
    return labels.join(" › ");
  };
  return products
    .map((taxon) => ({ slug: taxon.slug, label: breadcrumb(taxon.slug) }))
    .sort((a, b) => a.label.localeCompare(b.label));
};

// Audiences from the generated ClipDTOAudience enum, minus the "" (any) sentinel — "Any"
// is the Select's own placeholder value, not a listed option (Radix forbids an empty item
// value). Kinds likewise from ClipDTOKind: all six clip kinds are selectable.
const AUDIENCES = Object.values(ClipDTOAudience).filter((a): a is Exclude<typeof a, ""> => a !== "");
const KINDS = Object.values(ClipDTOKind);

const KIND_LABEL: Record<(typeof KINDS)[number], string> = {
  commercial: "Commercials",
  bumper: "Bumpers",
  station_id: "Station IDs",
  psa: "PSAs",
  trailer: "Trailers",
  interstitial: "Interstitials",
};

const AUDIENCE_LABEL: Record<(typeof AUDIENCES)[number], string> = {
  kids: "Kids",
  family: "Family",
  general: "General",
  late_night: "Late night",
};

// toggle — add/remove a value from a selection list, returning undefined for an empty
// result so the wire carries "any" (omitted) rather than an empty array. Keeps
// canonicalize's empty-means-any invariant honest at the source.
const toggle = (list: string[] | null | undefined, value: string): string[] | undefined => {
  const set = new Set(list ?? []);
  if (set.has(value)) set.delete(value);
  else set.add(value);
  return set.size === 0 ? undefined : [...set];
};

// FillerCriteria — the THEME half of the sandbox: era range, audience, categories, kinds.
// Controlled, like ChannelPolicyFields: the parent holds the draft and applies whatever
// `onChange` hands back. Every control clears to "any" when emptied (undefined, never a
// zero/empty-array that would read as a real, narrower restriction).
const FillerCriteria = ({
  selection,
  onChange,
  disabled,
  className,
  scopeEra,
  installationGeography,
}: {
  selection: FillerSelection;
  onChange: (next: FillerSelection) => void;
  disabled?: boolean;
  className?: string;
  // The CHANNEL's programming era, which an unset filler era inherits (§10 V51f). Passed in so
  // the inheritance can be SHOWN — it was applied live by the server and rendered nowhere, so a
  // channel drawing 1990s ads from a blank field looked like it was drawing from everything.
  scopeEra?: { from?: number; to?: number };
  installationGeography?: { country?: string; market?: string };
}) => {
  const era = selection.era;
  // ⚠ **Three states, and they are only distinguishable because `era` is a POINTER on the wire.**
  // Absent = inherit the channel's era; PRESENT-but-empty = explicitly any; a set range = itself.
  // Before V51f the first two were the same value, so "any era" was unreachable on any channel
  // that had a programming era — clearing the field simply re-inherited on the next derivation.
  const inheriting = era === undefined;
  const explicitlyAny = era !== undefined && !era.from && !era.to;
  const scopeLabel =
    scopeEra?.from && scopeEra.to
      ? `${scopeEra.from}–${scopeEra.to}`
      : (scopeEra?.from ?? scopeEra?.to)?.toString();
  const categories = selection.categories ?? [];
  const kinds = selection.kinds ?? [];
  const productCategories = useProductCategories();
  const [choosingCategories, setChoosingCategories] = useState(false);
  const fixedCountry = installationGeography?.country?.trim().toUpperCase();

  return (
    // Responsive 2-col grid: Era + Audience are cells; Categories (chip cloud) + Clip kinds
    // (checkbox row) span both columns.
    <div className={cn("grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2", className)}>
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel help="Country is a hard boundary. A local market also excludes local clips from every other market; timezone is never used as location.">
          Geography
        </FieldLabel>
        {selection.geography == null ? (
          <p className="text-muted-foreground text-xs" data-testid="geography-inherited">
            {installationGeography?.country
              ? `Following this installation (${installationGeography.country}${installationGeography.market ? ` · ${installationGeography.market}` : ""}).`
              : "No installation geography is configured; legacy unrestricted matching remains active."}{" "}
            <button
              type="button"
              className="text-signal underline-offset-2 hover:underline disabled:opacity-50"
              disabled={disabled}
              onClick={() =>
                onChange({
                  ...selection,
                  geography: {
                    country: installationGeography?.country ?? "US",
                    market: installationGeography?.market,
                  },
                })
              }
            >
              Set for this channel
            </button>
          </p>
        ) : (
          <>
            <div className="flex flex-wrap items-end gap-3">
              <div className="flex flex-col gap-1">
                <Label htmlFor="filler-country" className="text-muted-foreground text-xs">
                  Country code
                </Label>
                <Input
                  id="filler-country"
                  className="w-28 uppercase"
                  maxLength={2}
                  disabled={disabled || Boolean(fixedCountry)}
                  defaultValue={fixedCountry ?? selection.geography.country}
                  key={`country-${fixedCountry ?? selection.geography.country}`}
                  onBlur={(event) =>
                    onChange({
                      ...selection,
                      geography: {
                        ...selection.geography!,
                        country: fixedCountry ?? event.target.value.trim().toUpperCase(),
                      },
                    })
                  }
                />
              </div>
              <div className="flex min-w-52 flex-1 flex-col gap-1">
                <Label htmlFor="filler-market" className="text-muted-foreground text-xs">
                  Local market (optional)
                </Label>
                <Input
                  id="filler-market"
                  disabled={disabled}
                  defaultValue={selection.geography.market ?? ""}
                  key={`market-${selection.geography.market ?? ""}`}
                  placeholder="New York"
                  onBlur={(event) =>
                    onChange({
                      ...selection,
                      geography: {
                        ...selection.geography!,
                        country: fixedCountry ?? selection.geography!.country,
                        market: event.target.value.trim() || undefined,
                      },
                    })
                  }
                />
              </div>
            </div>
            <p className="text-muted-foreground text-xs">
              A blank market means country-wide national filler only.{" "}
              <button
                type="button"
                className="text-signal underline-offset-2 hover:underline disabled:opacity-50"
                disabled={disabled}
                onClick={() => {
                  const { geography: _dropped, ...rest } = selection;
                  onChange(rest);
                }}
              >
                Follow installation geography
              </button>
            </p>
          </>
        )}
      </div>
      {/* Era — two commit-on-blur year inputs, the same idiom as the program-scope era.
          Blank on either side means unbounded, not 0. Spans both columns (nested 2-up row). */}
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel help="Match commercials from this era. Left blank, it follows the channel's own era.">
          Era
        </FieldLabel>
        {/* ⚠ The inherited state is SHOWN rather than implied by an empty field (§10 V51f).
            The server has always applied `policy.scope.era` to an unset filler era, live on every
            derivation — but nothing said so, so a channel quietly drawing 1990s ads from two
            blank inputs read as "any era". Naming it is what makes the escape below make sense. */}
        {inheriting && scopeLabel && (
          <p className="text-muted-foreground text-xs" data-testid="era-inherited">
            Following the channel&rsquo;s era ({scopeLabel}).{" "}
            <button
              type="button"
              className="text-signal underline-offset-2 hover:underline disabled:opacity-50"
              disabled={disabled}
              // An EMPTY range, not a cleared field: presence is what tells the server the
              // operator answered "any" rather than not answering.
              onClick={() => onChange({ ...selection, era: {} })}
            >
              Use any era
            </button>
          </p>
        )}
        {explicitlyAny && scopeLabel && (
          <p className="text-muted-foreground text-xs" data-testid="era-any">
            Any era.{" "}
            <button
              type="button"
              className="text-signal underline-offset-2 hover:underline disabled:opacity-50"
              disabled={disabled}
              // Removing the key entirely is what "inherit" IS — the absence is the signal.
              onClick={() => {
                const { era: _dropped, ...rest } = selection;
                onChange(rest);
              }}
            >
              Follow the channel&rsquo;s era ({scopeLabel})
            </button>
          </p>
        )}
        <div className="flex items-center gap-3">
          <div className="flex flex-col gap-1">
            <Label htmlFor="filler-era-from" className="text-muted-foreground text-xs">
              From year
            </Label>
            <Input
              id="filler-era-from"
              type="number"
              className="w-28"
              disabled={disabled}
              defaultValue={era?.from ?? ""}
              placeholder="Any"
              key={`from-${era?.from ?? ""}`}
              onBlur={(e) => {
                const from = e.target.value === "" ? undefined : Number(e.target.value);
                if (from === era?.from) return;
                onChange({ ...selection, era: { ...era, from } });
              }}
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="filler-era-to" className="text-muted-foreground text-xs">
              To year
            </Label>
            <Input
              id="filler-era-to"
              type="number"
              className="w-28"
              disabled={disabled}
              defaultValue={era?.to ?? ""}
              placeholder="Any"
              key={`to-${era?.to ?? ""}`}
              onBlur={(e) => {
                const to = e.target.value === "" ? undefined : Number(e.target.value);
                if (to === era?.to) return;
                onChange({ ...selection, era: { ...era, to } });
              }}
            />
          </div>
        </div>
      </div>

      {/* Audience — "any" is the sentinel (Radix forbids an empty item value). */}
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel
          htmlFor="filler-audience"
          help="Keep breaks age-appropriate: kids' cartoons get kids' ads."
        >
          Audience
        </FieldLabel>
        <Select
          value={selection.audience || "any"}
          disabled={disabled}
          onValueChange={(v) => onChange({ ...selection, audience: v === "any" ? "" : v })}
        >
          <SelectTrigger id="filler-audience">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="any">Any audience</SelectItem>
            {AUDIENCES.map((a) => (
              <SelectItem key={a} value={a}>
                {AUDIENCE_LABEL[a]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Product/topic tags — a multi-select over the live product-axis hierarchy. No selection
          means any product/topic, the widest pool. */}
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel help="Narrow by what a clip is about. A broad tag such as Food includes every descendant; none selected draws from all products and topics.">
          Products & topics
        </FieldLabel>
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground text-sm">
            {categories.length === 0
              ? "All products & topics"
              : `${categories.length} ${categories.length === 1 ? "tag" : "tags"} selected`}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            aria-expanded={choosingCategories}
            onClick={() => setChoosingCategories((current) => !current)}
          >
            {choosingCategories ? "Done" : "Choose products & topics"}
          </Button>
        </div>
        {(choosingCategories || categories.length > 0) && (
          <div className="flex flex-wrap gap-1.5">
            {productCategories
              .filter((c) => choosingCategories || categories.includes(c.slug))
              .map((c) => {
                const on = categories.includes(c.slug);
                return (
                  <button
                    key={c.slug}
                    type="button"
                    disabled={disabled}
                    aria-pressed={on}
                    onClick={() => onChange({ ...selection, categories: toggle(categories, c.slug) })}
                    className={cn(
                      "cursor-pointer rounded-full border px-2.5 py-1 text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-50",
                      on
                        ? "border-signal bg-signal-tint-30 text-foreground"
                        : "border-border text-muted-foreground hover:border-input hover:text-foreground",
                    )}
                  >
                    {c.label}
                  </button>
                );
              })}
          </div>
        )}
      </div>

      {/* Kinds — checkboxes over the six clip kinds. None checked means the default set
          (commercials + bumpers + station IDs). */}
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel help="Which clips a break may use. None checked uses the default mix (commercials, bumpers, station IDs).">
          Clip kinds
        </FieldLabel>
        <div className="flex flex-wrap gap-x-4 gap-y-2">
          {KINDS.map((k) => (
            <label
              key={k}
              htmlFor={`filler-kind-${k}`}
              className="flex cursor-pointer items-center gap-2 text-sm"
            >
              <Checkbox
                id={`filler-kind-${k}`}
                checked={kinds.includes(k)}
                disabled={disabled}
                onChange={() => onChange({ ...selection, kinds: toggle(kinds, k) })}
              />
              {KIND_LABEL[k]}
            </label>
          ))}
        </div>
      </div>
    </div>
  );
};

export { AUDIENCE_LABEL, FillerCriteria, KIND_LABEL, toggle };
