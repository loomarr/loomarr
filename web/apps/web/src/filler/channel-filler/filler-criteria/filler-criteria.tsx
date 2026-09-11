import * as fillerApi from "@loomarr/api/endpoints/filler";
import { ClipDTOAudience } from "@loomarr/api/models/clipDTOAudience";
import { ClipDTOKind } from "@loomarr/api/models/clipDTOKind";
import type { DateScope } from "@loomarr/api/models/dateScope";
import type { FillerSelection } from "@loomarr/api/models/fillerSelection";
import type { Range } from "@loomarr/api/models/range";
import { useEffect, useState } from "react";
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
// value). Unclassified is a held lifecycle state rather than a selectable pod role.
const AUDIENCES = Object.values(ClipDTOAudience).filter((a): a is Exclude<typeof a, ""> => a !== "");
const KINDS = Object.values(ClipDTOKind).filter((kind) => kind !== "unclassified") as Exclude<
  (typeof ClipDTOKind)[keyof typeof ClipDTOKind],
  "unclassified"
>[];

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

const YEAR_MIN = 1900;
const YEAR_MAX = 2099;
const MAX_ERA_WINDOWS = 8;

const isValidWindow = (range: Range): range is Range & { from: number; to: number } =>
  Number.isInteger(range.from) &&
  Number.isInteger(range.to) &&
  range.from !== undefined &&
  range.to !== undefined &&
  range.from >= YEAR_MIN &&
  range.from <= YEAR_MAX &&
  range.to >= YEAR_MIN &&
  range.to <= YEAR_MAX &&
  range.from <= range.to;

const normalizeWindows = (windows: Range[] | undefined): Range[] => {
  const valid = (windows ?? []).filter(isValidWindow).sort((a, b) => a.from - b.from || a.to - b.to);
  return valid.reduce<Range[]>((merged, range) => {
    const previous = merged.at(-1);
    if (previous?.to !== undefined && range.from <= previous.to + 1)
      previous.to = Math.max(previous.to, range.to);
    else merged.push({ from: range.from, to: range.to });
    return merged;
  }, []);
};

const rangesLabel = (ranges: Range[] | undefined): string | undefined => {
  const normalized = normalizeWindows(ranges);
  return normalized.length ? normalized.map((range) => `${range.from}–${range.to}`).join(", ") : undefined;
};

// A legacy scalar scope can be open-ended. It is a display fallback only: multi-window
// data remains subject to the strict complete-window validation above.
const scalarScopeLabel = (scopeEra: { from?: number; to?: number } | undefined): string | undefined => {
  if (scopeEra?.from !== undefined && scopeEra.to !== undefined) return `${scopeEra.from}–${scopeEra.to}`;
  const year = scopeEra?.from ?? scopeEra?.to;
  return year === undefined ? undefined : String(year);
};

const WindowRow = ({
  window,
  index,
  disabled,
  onCommit,
  onRemove,
}: {
  window: Range;
  index: number;
  disabled?: boolean;
  onCommit: (index: number, next: Range) => void;
  onRemove: (index: number) => void;
}) => {
  const [draft, setDraft] = useState({ from: String(window.from), to: String(window.to) });

  useEffect(() => {
    setDraft({ from: String(window.from), to: String(window.to) });
  }, [window.from, window.to]);

  return (
    <div className="flex flex-wrap items-end gap-2">
      <div className="flex flex-col gap-1">
        <Label htmlFor={`filler-window-${index}-from`} className="text-muted-foreground text-xs">
          From year
        </Label>
        <Input
          id={`filler-window-${index}-from`}
          type="number"
          min={YEAR_MIN}
          max={YEAR_MAX}
          className="w-28"
          disabled={disabled}
          value={draft.from}
          onChange={(event) => setDraft({ ...draft, from: event.target.value })}
          onBlur={() => onCommit(index, { from: Number(draft.from), to: Number(draft.to) })}
        />
      </div>
      <div className="flex flex-col gap-1">
        <Label htmlFor={`filler-window-${index}-to`} className="text-muted-foreground text-xs">
          To year
        </Label>
        <Input
          id={`filler-window-${index}-to`}
          type="number"
          min={YEAR_MIN}
          max={YEAR_MAX}
          className="w-28"
          disabled={disabled}
          value={draft.to}
          onChange={(event) => setDraft({ ...draft, to: event.target.value })}
          onBlur={() => onCommit(index, { from: Number(draft.from), to: Number(draft.to) })}
        />
      </div>
      <Button type="button" variant="ghost" size="sm" disabled={disabled} onClick={() => onRemove(index)}>
        Remove
      </Button>
    </div>
  );
};

const WindowEditor = ({
  windows,
  onChange,
  disabled,
}: {
  windows: Range[];
  onChange: (next: Range[]) => void;
  disabled?: boolean;
}) => {
  const [draft, setDraft] = useState({ from: "", to: "" });
  const [error, setError] = useState<string>();
  const valid = isValidWindow;
  const commit = (index: number, next: Range) => {
    if (!valid(next)) {
      setError(`Enter whole years from ${YEAR_MIN} to ${YEAR_MAX}, with From no later than To.`);
      return;
    }
    setError(undefined);
    const changed = [...windows];
    changed[index] = next;
    onChange(normalizeWindows(changed));
  };
  const add = () => {
    const next = { from: Number(draft.from), to: Number(draft.to) };
    if (!draft.from || !draft.to || !valid(next)) {
      setError(`Enter whole years from ${YEAR_MIN} to ${YEAR_MAX}, with From no later than To.`);
      return;
    }
    setError(undefined);
    setDraft({ from: "", to: "" });
    onChange(normalizeWindows([...windows, next]));
  };
  return (
    <div className="flex flex-col gap-2">
      {windows.map((window, index) => (
        <WindowRow
          key={`${window.from}-${window.to}`}
          window={window}
          index={index}
          disabled={disabled}
          onCommit={commit}
          onRemove={(row) => onChange(windows.filter((_, current) => current !== row))}
        />
      ))}
      {windows.length < MAX_ERA_WINDOWS && (
        <div className="flex flex-wrap items-end gap-2">
          <Input
            aria-label="New date range from"
            type="number"
            min={YEAR_MIN}
            max={YEAR_MAX}
            className="w-28"
            disabled={disabled}
            value={draft.from}
            onChange={(event) => setDraft({ ...draft, from: event.target.value })}
            placeholder="From"
          />
          <Input
            aria-label="New date range to"
            type="number"
            min={YEAR_MIN}
            max={YEAR_MAX}
            className="w-28"
            disabled={disabled}
            value={draft.to}
            onChange={(event) => setDraft({ ...draft, to: event.target.value })}
            placeholder="To"
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="w-fit"
            disabled={disabled}
            onClick={add}
          >
            Add range
          </Button>
        </div>
      )}
      {error && (
        <p className="text-onair-300 text-xs" role="alert">
          {error}
        </p>
      )}
    </div>
  );
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
  programmingDates,
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
  programmingDates?: DateScope;
  installationGeography?: { country?: string; market?: string };
}) => {
  const era = selection.era;
  const eraWindows = selection.eraWindows;
  // ⚠ **Three states, and they are only distinguishable because `era` is a POINTER on the wire.**
  // Absent = inherit the channel's era; PRESENT-but-empty = explicitly any; a set range = itself.
  // Before V51f the first two were the same value, so "any era" was unreachable on any channel
  // that had a programming era — clearing the field simply re-inherited on the next derivation.
  const inheriting = era === undefined && eraWindows === undefined;
  const explicitlyAny = era !== undefined && !era.from && !era.to;
  const inheritedWindows = programmingDates
    ? normalizeWindows([
        ...(programmingDates.movieRelease ?? []),
        ...((programmingDates.seriesAiring?.length ?? 0) > 0
          ? (programmingDates.seriesAiring ?? [])
          : (programmingDates.seriesPremiere ?? [])),
      ])
    : undefined;
  const scopeLabel = rangesLabel(inheritedWindows) ?? scalarScopeLabel(scopeEra);
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
        {/* An absent selection inherits the programming dates. Presence is meaningful: an empty
            scalar is explicit-any, and windows are a separate explicit representation. */}
        {inheriting && scopeLabel && (
          <p className="text-muted-foreground text-xs" data-testid="era-inherited">
            Following the channel&rsquo;s era ({scopeLabel}).{" "}
            <button
              type="button"
              className="text-signal underline-offset-2 hover:underline disabled:opacity-50"
              disabled={disabled}
              onClick={() => {
                const { eraWindows: _windows, ...rest } = selection;
                onChange({ ...rest, era: {} });
              }}
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
              onClick={() => {
                const { era: _era, eraWindows: _windows, ...rest } = selection;
                onChange(rest);
              }}
            >
              Follow the channel&rsquo;s era ({scopeLabel})
            </button>
          </p>
        )}
        {eraWindows !== undefined ? (
          <>
            <p className="text-muted-foreground text-xs" data-testid="era-windows">
              Matching these date ranges only. Gaps remain excluded.
            </p>
            <WindowEditor
              windows={normalizeWindows(eraWindows)}
              disabled={disabled}
              onChange={(next) => {
                if (next.length === 0) {
                  const { eraWindows: _windows, ...rest } = selection;
                  onChange({ ...rest, era: {} });
                  return;
                }
                const { era: _era, ...rest } = selection;
                onChange({ ...rest, eraWindows: next });
              }}
            />
          </>
        ) : (
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
                  const { eraWindows: _windows, ...rest } = selection;
                  onChange({ ...rest, era: { ...era, from } });
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
                  const { eraWindows: _windows, ...rest } = selection;
                  onChange({ ...rest, era: { ...era, to } });
                }}
              />
            </div>
          </div>
        )}
        {eraWindows === undefined && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="w-fit"
            disabled={disabled}
            onClick={() => {
              const { era: _era, ...rest } = selection;
              onChange({ ...rest, eraWindows: [{ from: YEAR_MIN, to: YEAR_MAX }] });
            }}
          >
            Use date ranges
          </Button>
        )}
        {eraWindows !== undefined && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="w-fit"
            disabled={disabled}
            onClick={() => {
              const { eraWindows: _windows, ...rest } = selection;
              onChange({ ...rest, era: {} });
            }}
          >
            Use any era
          </Button>
        )}
        {!inheriting && !explicitlyAny && scopeLabel && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="w-fit"
            disabled={disabled}
            onClick={() => {
              const { era: _era, eraWindows: _windows, ...rest } = selection;
              onChange(rest);
            }}
          >
            Follow the channel&rsquo;s era ({scopeLabel})
          </Button>
        )}
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

      {/* Kinds — checkboxes over the six playable clip kinds. None checked means the default set
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
