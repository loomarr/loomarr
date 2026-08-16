import { SettingEntryProvenance } from "@loomarr/api/models/settingEntryProvenance";
import { SettingResultStatus } from "@loomarr/api/models/settingResultStatus";
import { formatRelative, humanizeSettingKey } from "@loomarr/core/format";
import { Lock, LockOpen, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { FieldHelp } from "../../feedback";
import type { SettingFieldProps } from "./setting-field.type";

// SettingField — one registry key as a form control (config-design §2, §3, §6). The
// same field powers the wizard and Settings; there is no parallel wizard form system.
// Everything it renders is contract data: `kind` picks the control, `enum` fills the
// options, `doc` is the help text, `provenance: env` LOCKS the field with a "set via
// environment" chip (env > database > default, visible provenance), and a secret shows
// its masked tail with replace-only editing (§4 — a stored secret is never echoed).
const inputTypeFor = (kind: string): string => {
  if (kind === "int") return "number";
  if (kind === "url") return "url";
  return "text";
};

type DurationUnit = "days" | "hours" | "minutes" | "seconds";

const durationParts = (raw: string): { amount: number; unit: DurationUnit } => {
  const factors: Record<string, number> = { h: 3600, m: 60, s: 1, ms: 0.001 };
  let seconds = 0;
  let matched = false;
  for (const match of raw.matchAll(/(-?\d+(?:\.\d+)?)(ms|h|m|s)/g)) {
    const factor = factors[match[2] ?? ""];
    if (factor === undefined) continue;
    matched = true;
    seconds += Number(match[1]) * factor;
  }
  if (!matched || !Number.isFinite(seconds)) return { amount: 0, unit: "hours" };
  if (seconds !== 0 && seconds % 86400 === 0) return { amount: seconds / 86400, unit: "days" };
  if (seconds % 3600 === 0) return { amount: seconds / 3600, unit: "hours" };
  if (seconds % 60 === 0) return { amount: seconds / 60, unit: "minutes" };
  return { amount: seconds, unit: "seconds" };
};

const durationRaw = (amount: number, unit: DurationUnit): string => {
  const seconds = amount * ({ days: 86400, hours: 3600, minutes: 60, seconds: 1 } as const)[unit];
  return `${Number(seconds.toFixed(3))}s`;
};

const byteParts = (raw: string): { amount: number; unit: "MiB" } => {
  const bytes = Number(raw);
  if (!Number.isFinite(bytes)) return { amount: 0, unit: "MiB" };
  return { amount: Number((bytes / 1024 ** 2).toFixed(2)), unit: "MiB" };
};

const SettingField = ({
  entry,
  value,
  onChange,
  result,
  compact,
  labelledBy,
  onEnvOverride,
  disabledReason,
  descriptionId,
  className,
}: SettingFieldProps) => {
  const [replacing, setReplacing] = useState(false);
  const id = `setting-${entry.key}`;
  const label = entry.label || humanizeSettingKey(entry.key);
  // `pinned` is the LOCK STATE, which is no longer the same question as "is this env's key".
  // An unlocked key (§3.1) resolves as `db` while its variable is still set, so it is
  // editable — but still overriding, which the badge below has to say.
  const pinned = entry.provenance === SettingEntryProvenance.env;
  const overriding = entry.envOverride === true;
  // Offer the affordance only where it can actually do something: the environment must set
  // the key, and the surface must have supplied a handler.
  const canUnlock = onEnvOverride !== undefined && entry.envPinnable === true;
  const invalid = result?.status === SettingResultStatus.invalid;
  // Compact mode renders neither explanatory element, so pointing at one would be a dangling
  // reference. Full fields associate both the ordinary help and any unavailable reason.
  const describedBy = compact
    ? undefined
    : [entry.doc ? `${id}-doc` : undefined, disabledReason ? `${id}-unavailable` : undefined, descriptionId]
        .filter(Boolean)
        .join(" ") || undefined;

  // A stored secret renders as its masked tail until the operator opts into replacing it.
  const secretLocked = entry.secret && entry.set && !replacing;

  const control = () => {
    if (entry.kind === "bool") {
      return (
        <Checkbox
          id={id}
          checked={value === "true"}
          disabled={pinned || disabledReason !== undefined}
          aria-describedby={describedBy}
          aria-labelledby={labelledBy}
          onChange={(e) => onChange(String(e.target.checked))}
        />
      );
    }
    if (entry.kind === "enum") {
      return (
        <Select value={value} disabled={pinned || disabledReason !== undefined} onValueChange={onChange}>
          <SelectTrigger
            id={id}
            aria-describedby={describedBy}
            aria-labelledby={labelledBy}
            aria-invalid={invalid ? "true" : undefined}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {/* Prefer the registry-owned {value,label} options (config-design §5) so the
                dropdown shows "OpenAI"/"Emby", not the raw stored value. Fall back to the
                bare value list for any enum that ships no labels. */}
            {(entry.enumOptions ?? (entry.enum ?? []).map((v) => ({ value: v, label: v }))).map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      );
    }
    if (entry.kind === "duration") {
      const current = durationParts(value);
      return (
        <div className="flex gap-2">
          <Input
            id={id}
            type="number"
            min={0}
            step="any"
            value={current.amount}
            disabled={pinned || disabledReason !== undefined}
            aria-describedby={describedBy}
            aria-labelledby={labelledBy}
            aria-invalid={invalid ? "true" : undefined}
            onChange={(e) => onChange(durationRaw(Number(e.target.value), current.unit))}
          />
          <Select
            value={current.unit}
            disabled={pinned || disabledReason !== undefined}
            onValueChange={(unit) => onChange(durationRaw(current.amount, unit as DurationUnit))}
          >
            <SelectTrigger aria-label={`${label} unit`} className="w-32 shrink-0">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(["days", "hours", "minutes", "seconds"] as const).map((unit) => (
                <SelectItem key={unit} value={unit}>
                  {unit}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      );
    }
    if (entry.presentation === "bytes") {
      const current = byteParts(value);
      return (
        <div className="flex gap-2">
          <Input
            id={id}
            type="number"
            min={0}
            step="any"
            value={current.amount}
            disabled={pinned || disabledReason !== undefined}
            aria-describedby={describedBy}
            aria-labelledby={labelledBy}
            aria-invalid={invalid ? "true" : undefined}
            onChange={(e) => {
              onChange(String(Math.round(Number(e.target.value) * 1024 ** 2)));
            }}
          />
          <span className="flex h-9 w-16 shrink-0 items-center justify-center rounded-md border border-input text-muted-foreground text-sm">
            {current.unit}
          </span>
        </div>
      );
    }
    return (
      <Input
        id={id}
        type={entry.secret ? "password" : inputTypeFor(entry.kind)}
        value={value}
        disabled={pinned || disabledReason !== undefined}
        autoComplete={entry.secret ? "new-password" : "off"}
        placeholder={entry.secret && entry.set ? "Enter a new value to replace" : undefined}
        aria-describedby={describedBy}
        aria-labelledby={labelledBy}
        aria-invalid={invalid ? "true" : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  };

  // Compact: the control alone. The secret replace-flow is kept — a stored secret must never
  // render its value, in a table cell least of all.
  if (compact) {
    return (
      <div className={cn("min-w-0", className)}>
        {secretLocked ? (
          <span className="font-mono text-muted-foreground text-xs">{entry.preview ?? "stored"}</span>
        ) : (
          control()
        )}
      </div>
    );
  }

  return (
    // `group` so the audit line can reveal on hover/focus of the whole field (below).
    <div className={cn("group flex flex-col gap-1.5", className)}>
      <div className="flex items-center gap-2">
        <Label htmlFor={id}>{label}</Label>
        {/* The one-line doc (§5 field anatomy) is present but moved into a hover (i) tooltip
            so the form isn't a wall of helper paragraphs. It stays programmatically associated
            via the sr-only doc below (aria-describedby), so screen readers still get it. */}
        {entry.doc && (
          // `describedById` points at the sr-only doc this component already renders below, so the
          // help prose lives in the DOM ONCE rather than once per carrier.
          <FieldHelp label={label} describedById={`${id}-doc`}>
            {entry.doc}
          </FieldHelp>
        )}
        {/* THE LOCK IS THE CONTROL (config-design §3.1). The chip already says why the field
            is read-only, so hanging a separate "unlock" button beside it would put the
            explanation and the way out in two places — the operator reads "set via
            environment", and the thing to click is somewhere else. Clicking the lock opens
            it; clicking the open lock hands the key back.

            Still a plain Badge when no handler is supplied: a read-only surface keeps the
            pre-3.1 chip exactly, rather than rendering a button that does nothing. */}
        {pinned &&
          (canUnlock ? (
            <button
              type="button"
              onClick={() => onEnvOverride(true)}
              // The visible text is the STATE; the action belongs in the accessible name, or
              // a screen-reader user hears "set via environment, button" and has to guess.
              aria-label={`Unlock ${label} to edit it here, currently set by ${entry.envVar ?? "the environment"}`}
              className="group/unlock cursor-pointer rounded-sm focus-visible:outline-2 focus-visible:outline-signal focus-visible:outline-offset-2"
            >
              {/* The hover/focus tell: the chip warms from inert grey to `signal`, and the
                  padlock springs open. `opacity-80` (what this was) says only "something is
                  here"; it never said the lock could be OPENED, which is the one thing an
                  operator needs to know before they can edit an env-pinned field.

                  ⚠ `signal`, NOT `lock`. `lock` is the GREEN success token — "checklist pass,
                  signal locked" (frontend-design §palette) — so colouring an *unlock*
                  affordance with it would claim success for an action not yet taken. `signal`
                  is the primary/interactive alias, which is what this is.

                  ⚠ The ICON SWAP is not decoration, it is the accessible half. A hover state
                  carried by colour alone excludes anyone who cannot distinguish the two, so
                  the shape has to change too — the same reason the connection dot pairs its
                  colour with a check/cross glyph.

                  ⚠ Driven by `group-focus-visible` as well as `group-hover`: a keyboard user
                  arrives here by Tab and would otherwise get a chip that never reacts.

                  ⚠ A NAMED group (`group/unlock`), and it has to be. The field wrapper is already
                  `.group` — it reveals the "changed by …" audit line on hover — and Tailwind's
                  bare `group-hover:` compiles to `:is(:where(.group):hover *)`, which matches ANY
                  hovered `.group` ancestor. Unnamed, the chip therefore lit up amber whenever the
                  pointer was anywhere in the field, promising that the label and the input were
                  clickable too. Naming the group scopes the tell to the chip itself.

                  200ms is the house duration for this kind of moment (frontend-design §7's
                  "lock in"). No reduced-motion guard needed — styles.css already zeroes every
                  transition under `prefers-reduced-motion`, globally. */}
              <Badge className="gap-1 transition-colors duration-200 group-hover/unlock:bg-signal-tint-15 group-hover/unlock:text-signal group-focus-visible/unlock:bg-signal-tint-15 group-focus-visible/unlock:text-signal">
                {/* Both padlocks occupy ONE fixed box so the crossfade cannot reflow the chip's
                    text — a swap that changes width would make the label twitch on hover. */}
                <span className="relative inline-flex size-3 shrink-0 items-center justify-center">
                  <Lock
                    className="absolute size-3 transition-all duration-200 group-hover/unlock:scale-90 group-hover/unlock:opacity-0 group-focus-visible/unlock:scale-90 group-focus-visible/unlock:opacity-0"
                    aria-hidden
                  />
                  <LockOpen
                    className="absolute size-3 scale-90 opacity-0 transition-all duration-200 group-hover/unlock:scale-100 group-hover/unlock:opacity-100 group-focus-visible/unlock:scale-100 group-focus-visible/unlock:opacity-100"
                    aria-hidden
                  />
                </span>
                set via environment
              </Badge>
            </button>
          ) : (
            <Badge className="gap-1">
              <Lock className="size-3" aria-hidden />
              set via environment
            </Badge>
          ))}
        {/* Overriding is a DIFFERENT state from both locked and ordinary-db, and it names the
            variable: "set via environment" on an editable field would contradict itself, and a
            bare `db` chip would imply the environment never mentioned this key. */}
        {overriding &&
          (canUnlock ? (
            <button
              type="button"
              onClick={() => onEnvOverride(false)}
              aria-label={`Hand ${label} back to ${entry.envVar ?? "the environment"}, your saved value is kept`}
              className="cursor-pointer rounded-full transition-opacity hover:opacity-80"
            >
              <Badge variant="caution" className="gap-1">
                <LockOpen className="size-3" aria-hidden />
                overriding {entry.envVar ?? "the environment"}
              </Badge>
            </button>
          ) : (
            <Badge variant="caution" className="gap-1">
              <LockOpen className="size-3" aria-hidden />
              overriding {entry.envVar ?? "the environment"}
            </Badge>
          ))}
        {result?.status === SettingResultStatus.pinned && !pinned && (
          <Badge variant="caution">not saved: pinned</Badge>
        )}
      </div>

      {secretLocked ? (
        <div className="flex items-center gap-2">
          <span className="flex h-9 flex-1 items-center rounded-md border border-input px-3 font-mono text-muted-foreground text-sm">
            {entry.preview ?? "stored"}
          </span>
          <button
            type="button"
            onClick={() => {
              setReplacing(true);
              onChange("");
            }}
            className="cursor-pointer rounded-md border border-input px-3 py-1.5 text-sm transition-colors hover:bg-accent"
          >
            Replace
          </button>
        </div>
      ) : (
        control()
      )}

      {/* The doc, kept in the DOM for `aria-describedby` (screen readers) but visually hidden —
          the visible affordance is the FieldHelp (i) tooltip in the label row. */}
      {entry.doc && (
        <p id={`${id}-doc`} className="sr-only">
          {entry.doc}
        </p>
      )}

      {entry.caution && (
        <p className="flex items-center gap-1 text-onair-300 text-xs">
          <TriangleAlert className="size-3" aria-hidden />
          The stored value was invalid and has been reset to the default.
        </p>
      )}

      {disabledReason && (
        <p id={`${id}-unavailable`} role="alert" className="flex items-start gap-1 text-onair-300 text-xs">
          <TriangleAlert className="mt-0.5 size-3 shrink-0" aria-hidden />
          {disabledReason}
        </p>
      )}

      {/* "changed by … · when" (§5 field anatomy). Audit provenance is signal when you're
          auditing, noise at rest — so it's revealed on hover/focus of the field rather than
          costing a permanent line under every control (config-design §5). It stays in the DOM
          (opacity, not conditional render) so it's reachable by keyboard (focus-within) and
          screen readers; the transition is frozen under reduced-motion. Only for a value a
          PERSON set: an env pin or built-in default has no author to name. */}
      {entry.updatedAt && (
        <p className="pointer-events-none text-muted-foreground text-xs opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100 motion-reduce:transition-none">
          {entry.updatedBy ? `Changed by ${entry.updatedBy} · ` : "Changed "}
          {formatRelative(entry.updatedAt)}
        </p>
      )}

      {invalid && result?.problem && (
        <p role="alert" className="text-onair-300 text-xs">
          {result.problem}
        </p>
      )}
    </div>
  );
};

export { SettingField };
