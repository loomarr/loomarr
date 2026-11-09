import * as fillerApi from "@loomarr/api/endpoints/filler";
import { ClipDTOAudience } from "@loomarr/api/models/clipDTOAudience";
import { ClipDTORole } from "@loomarr/api/models/clipDTORole";
import type { DateScope } from "@loomarr/api/models/dateScope";
import type { FillerSelection } from "@loomarr/api/models/fillerSelection";
import type { Range } from "@loomarr/api/models/range";
import { Action, CompactSelect, Text, Toggle } from "@loomarr/design-system";
import { CommitField, normalizeYearRanges, YEAR_MAX, YEAR_MIN, YearRangeListField } from "@loomarr/ui";
import { useState } from "react";
import { FieldHelp } from "@/components/loomarr/feedback/field-help";
import { LocationPicker, usePlaceLabel } from "@/components/loomarr/settings/installation-location";
import { cn } from "@/lib/utils";

// FieldLabel — a label + (i) help icon, replacing the permanent helper `<p>` under each
// control (mirrors ChannelPolicyFields). Help on hover keeps the criteria form compact.
const FieldLabel = ({ htmlFor, children, help }: { htmlFor?: string; children: string; help: string }) => (
  <div className="flex items-center gap-1.5">
    <Text as="label" htmlFor={htmlFor} textRole="label">
      {children}
    </Text>
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
const ROLES = Object.values(ClipDTORole).filter((role) => role !== "unclassified") as Exclude<
  (typeof ClipDTORole)[keyof typeof ClipDTORole],
  "unclassified"
>[];

const ROLE_LABEL: Record<(typeof ROLES)[number], string> = {
  commercial: "Commercials",
  bumper: "Bumpers",
  station_id: "Station IDs",
  psa: "PSAs",
  trailer: "Trailers",
  interstitial: "Interstitials",
  political: "Political ads",
  news: "News & weather",
};

const AUDIENCE_LABEL: Record<(typeof AUDIENCES)[number], string> = {
  kids: "Kids",
  family: "Family",
  general: "General",
  late_night: "Late night",
};

const AUDIENCE_OPTIONS = [
  { label: "Any audience", value: "" },
  ...AUDIENCES.map((audience) => ({ label: AUDIENCE_LABEL[audience], value: audience })),
];

// toggle — add/remove a value from a selection list, returning undefined for an empty
// result so the wire carries "any" (omitted) rather than an empty array. Keeps
// canonicalize's empty-means-any invariant honest at the source.
const toggle = (list: string[] | null | undefined, value: string): string[] | undefined => {
  const set = new Set(list ?? []);
  if (set.has(value)) set.delete(value);
  else set.add(value);
  return set.size === 0 ? undefined : [...set];
};

// toggleRole — toggle a clip role against what the checklist SHOWS. No roles chosen shows the
// server's default set ticked (#2037), so the first tick starts from it rather than from nothing;
// a set that matches the default again saves as no choice, so the channel keeps following it.
const toggleRole = (shown: string[], value: string, defaults: string[]): string[] | undefined => {
  const next = toggle(shown, value);
  const isDefault =
    next !== undefined && next.length === defaults.length && defaults.every((role) => next.includes(role));
  return isDefault ? undefined : next;
};

const MAX_ERA_WINDOWS = 8;

const rangesLabel = (ranges: Range[] | undefined): string | undefined => {
  const normalized = normalizeYearRanges(ranges);
  return normalized.length ? normalized.map((range) => `${range.from}–${range.to}`).join(", ") : undefined;
};

// A legacy scalar scope can be open-ended. It is a display fallback only: multi-window
// data remains subject to normalizeYearRanges's strict complete-range validation.
const scalarScopeLabel = (scopeEra: { from?: number; to?: number } | undefined): string | undefined => {
  if (scopeEra?.from !== undefined && scopeEra.to !== undefined) return `${scopeEra.from}–${scopeEra.to}`;
  const year = scopeEra?.from ?? scopeEra?.to;
  return year === undefined ? undefined : String(year);
};

// FillerCriteria — the THEME half of the sandbox: era range, audience, categories, roles.
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
  defaultRoles,
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
  installationGeography?: { country?: string; place?: string };
  // The roles a channel that chose none airs, from the server's coverage answer. Until it arrives
  // the checklist cannot show the default, so it stays disabled rather than narrowing from nothing.
  defaultRoles?: string[];
}) => {
  const installationPlaceLabel = usePlaceLabel(installationGeography?.place);
  const era = selection.era;
  const eraWindows = selection.eraWindows;
  // ⚠ **Three states, and they are only distinguishable because `era` is a POINTER on the wire.**
  // Absent = inherit the channel's era; PRESENT-but-empty = explicitly any; a set range = itself.
  // Before V51f the first two were the same value, so "any era" was unreachable on any channel
  // that had a programming era — clearing the field simply re-inherited on the next derivation.
  const inheriting = era === undefined && eraWindows === undefined;
  const explicitlyAny = era !== undefined && !era.from && !era.to;
  const inheritedWindows = programmingDates
    ? normalizeYearRanges([
        ...(programmingDates.movieRelease ?? []),
        ...((programmingDates.seriesAiring?.length ?? 0) > 0
          ? (programmingDates.seriesAiring ?? [])
          : (programmingDates.seriesPremiere ?? [])),
      ])
    : undefined;
  const scopeLabel = rangesLabel(inheritedWindows) ?? scalarScopeLabel(scopeEra);
  const categories = selection.categories ?? [];
  const chosenRoles = selection.roles ?? [];
  const roles = chosenRoles.length > 0 ? chosenRoles : (defaultRoles ?? []);
  const rolesUnknown = chosenRoles.length === 0 && defaultRoles === undefined;
  const productCategories = useProductCategories();
  const [choosingCategories, setChoosingCategories] = useState(false);
  const fixedCountry = installationGeography?.country?.trim().toUpperCase();

  return (
    // Responsive 2-col grid: Era + Audience are cells; Categories (chip cloud) + Clip kinds
    // (checkbox row) span both columns.
    <div className={cn("grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2", className)}>
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel help="Country is a hard boundary. A place also limits local clips to TV stations near it; timezone is never used as location.">
          Geography
        </FieldLabel>
        {selection.geography == null ? (
          <p className="text-muted-foreground text-xs" data-testid="geography-inherited">
            {installationGeography?.country
              ? `Following this installation (${installationGeography.country}${installationPlaceLabel ? ` · ${installationPlaceLabel}` : ""}).`
              : "No installation geography is configured; legacy unrestricted matching remains active."}{" "}
            <button
              type="button"
              className="text-signal underline-offset-2 hover:underline disabled:opacity-50"
              // Inline 12 px text: a touch screen gets the 24 px tap area (styles.css, #1785).
              data-touch-target
              disabled={disabled}
              onClick={() =>
                onChange({
                  ...selection,
                  // The place stays empty, so the channel keeps following the installation's.
                  geography: { country: installationGeography?.country ?? "US" },
                })
              }
            >
              Set for this channel
            </button>
          </p>
        ) : (
          <>
            <div className="flex flex-wrap items-end gap-3">
              <CommitField
                autoCapitalize="characters"
                disabled={disabled || Boolean(fixedCountry)}
                id="filler-country"
                initial={fixedCountry ?? selection.geography.country ?? ""}
                key={`country-${fixedCountry ?? selection.geography.country}`}
                label="Country code"
                maxLength={2}
                onCommit={(text) =>
                  onChange({
                    ...selection,
                    geography: {
                      ...selection.geography!,
                      country: fixedCountry ?? text.trim().toUpperCase(),
                    },
                  })
                }
                width={112}
              />
              <div className="min-w-52 flex-1">
                <LocationPicker
                  value={{
                    country: fixedCountry ?? selection.geography.country,
                    place: selection.geography.place,
                  }}
                  restrictCountry={fixedCountry}
                  locked={disabled}
                  onChange={(location) =>
                    onChange({
                      ...selection,
                      geography: {
                        ...selection.geography!,
                        country: fixedCountry ?? location.country,
                        place: location.place || undefined,
                      },
                    })
                  }
                />
              </div>
            </div>
            <p className="text-muted-foreground text-xs">
              A blank place follows the installation's city; with no city, only national filler airs.{" "}
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
            <YearRangeListField
              disabled={disabled}
              label="Era"
              maxRanges={MAX_ERA_WINDOWS}
              ranges={normalizeYearRanges(eraWindows)}
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
            <CommitField
              disabled={disabled}
              id="filler-era-from"
              initial={String(era?.from ?? "")}
              key={`from-${era?.from ?? ""}`}
              keyboardType="number-pad"
              label="From year"
              maxLength={4}
              onCommit={(text) => {
                const from = text === "" ? undefined : Number(text);
                if (from === era?.from) return;
                const { eraWindows: _windows, ...rest } = selection;
                onChange({ ...rest, era: { ...era, from } });
              }}
              placeholder="Any"
              width={112}
            />
            <CommitField
              disabled={disabled}
              id="filler-era-to"
              initial={String(era?.to ?? "")}
              key={`to-${era?.to ?? ""}`}
              keyboardType="number-pad"
              label="To year"
              maxLength={4}
              onCommit={(text) => {
                const to = text === "" ? undefined : Number(text);
                if (to === era?.to) return;
                const { eraWindows: _windows, ...rest } = selection;
                onChange({ ...rest, era: { ...era, to } });
              }}
              placeholder="Any"
              width={112}
            />
          </div>
        )}
        {eraWindows === undefined && (
          <Action
            disabled={disabled}
            onPress={() => {
              const { era: _era, ...rest } = selection;
              onChange({ ...rest, eraWindows: [{ from: YEAR_MIN, to: YEAR_MAX }] });
            }}
            style={{ alignSelf: "flex-start" }}
            variant="outline"
          >
            Use date ranges
          </Action>
        )}
        {eraWindows !== undefined && (
          <Action
            disabled={disabled}
            onPress={() => {
              const { eraWindows: _windows, ...rest } = selection;
              onChange({ ...rest, era: {} });
            }}
            style={{ alignSelf: "flex-start" }}
            variant="outline"
          >
            Use any era
          </Action>
        )}
        {!inheriting && !explicitlyAny && scopeLabel && (
          <Action
            disabled={disabled}
            onPress={() => {
              const { era: _era, eraWindows: _windows, ...rest } = selection;
              onChange(rest);
            }}
            style={{ alignSelf: "flex-start" }}
            variant="ghost"
          >
            Follow the channel&rsquo;s era ({scopeLabel})
          </Action>
        )}
      </div>

      {/* Audience — the empty value is "any audience". */}
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel
          htmlFor="filler-audience"
          help="Keep breaks age-appropriate: kids' cartoons get kids' ads."
        >
          Audience
        </FieldLabel>
        <CompactSelect
          disabled={disabled}
          id="filler-audience"
          label="Audience"
          onValueChange={(audience) => onChange({ ...selection, audience })}
          options={AUDIENCE_OPTIONS}
          value={selection.audience ?? ""}
        />
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
          <Action
            aria-expanded={choosingCategories}
            disabled={disabled}
            onPress={() => setChoosingCategories((current) => !current)}
            variant="outline"
          >
            {choosingCategories ? "Done" : "Choose products & topics"}
          </Action>
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
                      // Pressed is neutral (#1909 Q2): amber is focus or the primary action.
                      on
                        ? "border-foreground bg-static-800 text-foreground font-semibold"
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

      {/* Roles — checkboxes over the playable clip roles. A channel that chose none shows the
          server's default ticked: every role except political ads and news (#2037). */}
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel help="Which clips a break may use. Political ads and news stay off until you tick them; a nearby station's own news airs with its station IDs.">
          Clip kinds
        </FieldLabel>
        <div className="flex flex-wrap gap-x-4 gap-y-2">
          {ROLES.map((r) => (
            <Toggle
              key={r}
              id={`filler-role-${r}`}
              label={ROLE_LABEL[r]}
              checked={roles.includes(r)}
              disabled={disabled || rolesUnknown}
              onCheckedChange={() =>
                onChange({ ...selection, roles: toggleRole(roles, r, defaultRoles ?? []) })
              }
            />
          ))}
        </div>
        {rolesUnknown && (
          <p className="text-muted-foreground text-xs">
            Clip kinds unlock once this channel's coverage check loads.
          </p>
        )}
      </div>
    </div>
  );
};

export { FillerCriteria };
