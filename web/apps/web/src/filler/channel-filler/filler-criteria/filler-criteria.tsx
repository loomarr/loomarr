import { ClipDTOAudience, ClipDTOKind, type FillerSelection } from "@loomarr/api";
import {
  Checkbox,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import { cn } from "@/lib";

// The filler category closed set — MIRRORS internal/schedule/policy.go `fillerCategories`
// (the backend validates against it; `category` is a free string in the DTO, not a
// generated enum, so it has no orval counterpart to import). Keep this in sync if the
// backend set changes — a new category here without one there would 422 on apply.
const CATEGORIES = [
  "toys",
  "cereal",
  "cars",
  "tech",
  "fast_food",
  "movie_trailer",
  "candy",
  "games",
  "psa",
  "ident",
  "bumper",
  "general",
] as const;

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

// prettyCategory — "fast_food" → "Fast food" for the chip label, without a per-value map.
const prettyCategory = (c: string): string => c.replace(/_/g, " ").replace(/^./, (m) => m.toUpperCase());

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
}: {
  selection: FillerSelection;
  onChange: (next: FillerSelection) => void;
  disabled?: boolean;
  className?: string;
}) => {
  const era = selection.era;
  const categories = selection.categories ?? [];
  const kinds = selection.kinds ?? [];

  return (
    <div className={cn("flex flex-col gap-5", className)}>
      {/* Era — two commit-on-blur year inputs, the same idiom as the program-scope era.
          Blank on either side means unbounded, not 0. */}
      <div className="flex flex-col gap-1.5">
        <Label>Era</Label>
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
        <p className="text-muted-foreground text-xs">
          Match commercials from this era. Left blank, it follows the channel's own era.
        </p>
      </div>

      {/* Audience — "any" is the sentinel (Radix forbids an empty item value). */}
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="filler-audience">Audience</Label>
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
        <p className="text-muted-foreground text-xs">
          Keep breaks age-appropriate — kids' cartoons get kids' ads.
        </p>
      </div>

      {/* Categories — a multi-select of removable chips over the closed set. No selection
          means "any category", the widest pool. */}
      <div className="flex flex-col gap-1.5">
        <Label>Categories</Label>
        <div className="flex flex-wrap gap-1.5">
          {CATEGORIES.map((c) => {
            const on = categories.includes(c);
            return (
              <button
                key={c}
                type="button"
                disabled={disabled}
                aria-pressed={on}
                onClick={() => onChange({ ...selection, categories: toggle(categories, c) })}
                className={cn(
                  "cursor-pointer rounded-full border px-2.5 py-1 text-xs transition-colors disabled:cursor-not-allowed disabled:opacity-50",
                  on
                    ? "border-signal bg-signal-tint-30 text-foreground"
                    : "border-border text-muted-foreground hover:border-input hover:text-foreground",
                )}
              >
                {prettyCategory(c)}
              </button>
            );
          })}
        </div>
        <p className="text-muted-foreground text-xs">
          Narrow to certain kinds of ad. None selected draws from every category.
        </p>
      </div>

      {/* Kinds — checkboxes over the six clip kinds. None checked means the default set
          (commercials + bumpers + station IDs), spelled out in the caption. */}
      <div className="flex flex-col gap-1.5">
        <Label>Clip kinds</Label>
        <div className="flex flex-wrap gap-x-4 gap-y-2">
          {KINDS.map((k) => (
            <label key={k} className="flex cursor-pointer items-center gap-2 text-sm">
              <Checkbox
                checked={kinds.includes(k)}
                disabled={disabled}
                onChange={() => onChange({ ...selection, kinds: toggle(kinds, k) })}
              />
              {KIND_LABEL[k]}
            </label>
          ))}
        </div>
        <p className="text-muted-foreground text-xs">
          Which clips a break may use. None checked uses the default mix (commercials, bumpers, station IDs).
        </p>
      </div>
    </div>
  );
};

export { AUDIENCE_LABEL, CATEGORIES, FillerCriteria, KIND_LABEL, prettyCategory, toggle };
