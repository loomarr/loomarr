import { Input, Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui";
import { cn } from "@/lib";
import { FieldHelp } from "../field-help";
import type { ChannelPolicyFieldsProps } from "./channel-policy-fields.type";

// FieldLabel — a form label paired with an (i) help icon (FieldHelp), the row that replaces
// the old permanent helper `<p>` under every control. Keeps the form compact: the guidance is
// on hover, not always on screen. `htmlFor` ties the label to its control; the help names the
// same field for screen readers.
const FieldLabel = ({ htmlFor, children, help }: { htmlFor?: string; children: string; help: string }) => (
  <div className="flex items-center gap-1.5">
    <Label htmlFor={htmlFor}>{children}</Label>
    <FieldHelp label={children}>{help}</FieldHelp>
  </div>
);

// ChannelPolicyFields — a ChannelPolicy as plain-language editable chips (programming-design
// §8). Controlled: the parent holds `policy` and persists whatever this calls `onChange`
// with. `applied` is reconcile-owned (§9) and never rendered here — it is read-only history
// of what the scheduler had to ease, not a setting an operator edits. A field left blank
// clears the window entirely (undefined — "no restriction"), not zero.

// A no-repeat window is a Go-duration STRING on the wire ("168h", "30h0m0s"). For display
// we tidy the zero units the backend's Duration.String() emits: "30h0m0s" → "30h", "0s" →
// "" (no restriction). Non-duration text is left as-typed so a bad value is visible, not
// silently dropped.
const tidyDuration = (v: string | undefined): string => {
  if (!v || v === "0s") return "";
  const m = v.match(/^(\d+h)?(\d+m)?(\d+s)?$/);
  if (!m || (!m[1] && !m[2] && !m[3])) return v;
  const trimmed = [m[1], m[2], m[3]].filter((u) => u && !/^0[hms]$/.test(u)).join("");
  return trimmed || v;
};

// On save: pass the typed duration straight through (the backend accepts "24h"/"168h"),
// or undefined for blank — undefined = "no restriction", never "0" (which would mean the
// real, different thing "never repeat"). Whitespace-only is treated as blank.
const durationOrUndefined = (v: string): string | undefined => {
  const trimmed = v.trim();
  return trimmed === "" ? undefined : trimmed;
};

const ORDERING_OPTIONS: { value: string; label: string }[] = [
  { value: "inherit", label: "Inherit channel default" },
  { value: "sequential", label: "In order" },
  { value: "shuffle", label: "Shuffled" },
  { value: "syndication", label: "Syndication" },
];

const CEILING_OPTIONS: { value: string; label: string }[] = [
  { value: "none", label: "No limit" },
  { value: "TV-Y", label: "TV-Y" },
  { value: "TV-Y7", label: "TV-Y7" },
  { value: "TV-G", label: "TV-G" },
  { value: "TV-PG", label: "TV-PG" },
  { value: "TV-14", label: "TV-14" },
  { value: "TV-MA", label: "TV-MA" },
  { value: "G", label: "G" },
  { value: "PG", label: "PG" },
  { value: "PG-13", label: "PG-13" },
  { value: "R", label: "R" },
];

const ChannelPolicyFields = ({ policy, onChange, className }: ChannelPolicyFieldsProps) => {
  const era = policy.scope?.era;
  const separation = policy.separation;

  return (
    // A responsive 2-column field grid (was a 1-wide stack): the two Selects sit side by side,
    // while Era + No-repeat span the full width because they each hold a nested 2-up input row.
    // gap-x for columns, gap-y for rows.
    <div className={cn("grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2", className)}>
      {/* Ordering — empty string means "inherit the channel's Strategy". Radix Select
          forbids an empty-string item value, so "inherit" is the sentinel. */}
      <div className="flex flex-col gap-1.5">
        <FieldLabel htmlFor="policy-ordering" help="How programs are sequenced on this channel.">
          Ordering
        </FieldLabel>
        <Select
          value={policy.ordering || "inherit"}
          onValueChange={(v) => onChange({ ...policy, ordering: v === "inherit" ? "" : v })}
        >
          <SelectTrigger id="policy-ordering">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {ORDERING_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Audience ceiling — a safety limit, never relaxed by the ladder (§8), so the
          help says so explicitly rather than leaving that guarantee implicit. */}
      <div className="flex flex-col gap-1.5">
        <FieldLabel
          htmlFor="policy-ceiling"
          help="Content stays at or below this — a safety limit Loomarr never loosens."
        >
          Audience ceiling
        </FieldLabel>
        <Select
          value={policy.audience?.ceiling || "none"}
          onValueChange={(v) =>
            onChange({
              ...policy,
              audience: { ...policy.audience, ceiling: v === "none" ? "" : v },
            })
          }
        >
          <SelectTrigger id="policy-ceiling">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CEILING_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Era — two commit-on-blur year inputs. Blank on either side means unbounded, not 0.
          Spans both columns (it has its own nested 2-up row). */}
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel help="Restrict content to titles released in this range. Leave blank for no restriction.">
          Era
        </FieldLabel>
        <div className="flex items-center gap-3">
          <div className="flex flex-col gap-1">
            <Label htmlFor="policy-era-from" className="text-muted-foreground text-xs">
              From year
            </Label>
            <Input
              id="policy-era-from"
              type="number"
              className="w-28"
              defaultValue={era?.from ?? ""}
              placeholder="Any"
              onBlur={(e) => {
                const next = e.target.value === "" ? undefined : Number(e.target.value);
                if (next === era?.from) return;
                onChange({
                  ...policy,
                  scope: { ...policy.scope, era: { ...era, from: next } },
                });
              }}
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="policy-era-to" className="text-muted-foreground text-xs">
              To year
            </Label>
            <Input
              id="policy-era-to"
              type="number"
              className="w-28"
              defaultValue={era?.to ?? ""}
              placeholder="Any"
              onBlur={(e) => {
                const next = e.target.value === "" ? undefined : Number(e.target.value);
                if (next === era?.to) return;
                onChange({
                  ...policy,
                  scope: { ...policy.scope, era: { ...era, to: next } },
                });
              }}
            />
          </div>
        </div>
      </div>

      {/* No-repeat windows — commit-on-blur duration strings. Spans both columns (nested
          2-up row). */}
      <div className="flex flex-col gap-1.5 sm:col-span-2">
        <FieldLabel help="How long before the same title can play again (e.g. 168h = 7 days).">
          No-repeat windows
        </FieldLabel>
        <div className="flex items-center gap-3">
          <div className="flex flex-col gap-1">
            <Label htmlFor="policy-movie-norepeat" className="text-muted-foreground text-xs">
              Movies
            </Label>
            <Input
              id="policy-movie-norepeat"
              className="w-28"
              defaultValue={tidyDuration(separation?.movieNoRepeat)}
              placeholder="e.g. 168h"
              onBlur={(e) => {
                const next = durationOrUndefined(e.target.value);
                if (next === tidyDuration(separation?.movieNoRepeat) || next === separation?.movieNoRepeat)
                  return;
                onChange({ ...policy, separation: { ...separation, movieNoRepeat: next } });
              }}
            />
          </div>
          <div className="flex flex-col gap-1">
            <Label htmlFor="policy-episode-norepeat" className="text-muted-foreground text-xs">
              Episodes
            </Label>
            <Input
              id="policy-episode-norepeat"
              className="w-28"
              defaultValue={tidyDuration(separation?.episodeNoRepeat)}
              placeholder="e.g. 24h"
              onBlur={(e) => {
                const next = durationOrUndefined(e.target.value);
                if (
                  next === tidyDuration(separation?.episodeNoRepeat) ||
                  next === separation?.episodeNoRepeat
                )
                  return;
                onChange({ ...policy, separation: { ...separation, episodeNoRepeat: next } });
              }}
            />
          </div>
        </div>
      </div>
    </div>
  );
};

export { ChannelPolicyFields };
