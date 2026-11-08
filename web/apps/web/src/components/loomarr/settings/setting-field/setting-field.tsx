import { SettingEntryProvenance } from "@loomarr/api/models/settingEntryProvenance";
import { SettingKind } from "@loomarr/api/models/settingKind";
import { SettingPresentation } from "@loomarr/api/models/settingPresentation";
import { SettingResultStatus } from "@loomarr/api/models/settingResultStatus";
import { formatRelative, humanizeSettingKey } from "@loomarr/core/format";
import { Badge, Field, FieldHint, Surface, Toggle } from "@loomarr/design-system";
import { NumberField } from "@loomarr/ui";
import { Lock, LockOpen, TriangleAlert } from "lucide-react";
import { type ComponentProps, type ReactNode, useState } from "react";
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
const keyboardFor = (kind: SettingKind): ComponentProps<typeof Field>["keyboardType"] => {
  if (kind === SettingKind.int) return "number-pad";
  if (kind === SettingKind.url) return "url";
  return "default";
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

// The Custom choice offered after the registry's presets. It cannot collide with a preset: preset
// values are canonical setting values, and no setting accepts this word as one.
const CUSTOM_PRESET = "custom";

const byteParts = (raw: string): { amount: number; unit: "MiB" } => {
  const bytes = Number(raw);
  if (!Number.isFinite(bytes)) return { amount: 0, unit: "MiB" };
  return { amount: Number((bytes / 1024 ** 2).toFixed(2)), unit: "MiB" };
};

// A number box with its unit beside it: the bytes editor's MiB, or a registry-declared unit.
const WithUnit = ({ unit, children }: { unit: string; children: ReactNode }) => (
  <div className="flex gap-2">
    {children}
    <span className="border-input text-muted-foreground flex h-10 w-16 shrink-0 items-center justify-center rounded-md border text-sm">
      {unit}
    </span>
  </div>
);

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
  const [editingCustomSchedule, setEditingCustomSchedule] = useState(false);
  const id = `setting-${entry.key}`;
  const label = entry.label || humanizeSettingKey(entry.key);
  // `pinned` is the LOCK STATE, which is no longer the same question as "is this env's key".
  // An unlocked key (§3.1) resolves as `db` while its variable is still set, so it is
  // editable — but still overriding, which the badge below has to say.
  const pinned = entry.provenance === SettingEntryProvenance.env;
  // ⚠ A pinned VALUE is real, so it is shown read-only at full strength, never in the dimmed
  // disabled look that reads as an empty placeholder (#2759). Text boxes take `readOnly`; a
  // select stays disabled (it has no read-only state) but keeps body-strength text.
  const pinnedSelect = pinned ? "disabled:opacity-100" : undefined;
  const overriding = entry.envOverride === true;
  // Offer the affordance only where it can actually do something: the environment must set
  // the key, and the surface must have supplied a handler.
  const canUnlock = onEnvOverride !== undefined && entry.envPinnable === true;
  const invalid = result?.status === SettingResultStatus.invalid;
  const inlineSwitch = entry.kind === SettingKind.bool && entry.presentation === SettingPresentation.switch;
  const problem = invalid ? result?.problem : undefined;
  // Compact mode renders none of the explanatory elements, so pointing at one would be a dangling
  // reference. Full fields associate the ordinary help, any unavailable reason, and the server's
  // validation problem: `role="alert"` announces that once as it appears, and the description is
  // what a screen reader repeats when the operator comes back to the field to fix it.
  const describedBy = compact
    ? undefined
    : [
        entry.doc ? `${id}-doc` : undefined,
        disabledReason ? `${id}-unavailable` : undefined,
        problem ? `${id}-problem` : undefined,
        descriptionId,
      ]
        .filter(Boolean)
        .join(" ") || undefined;

  // A stored secret renders as its masked tail until the operator opts into replacing it.
  const secretLocked = entry.secret && entry.set && !replacing;

  const control = () => {
    if (entry.kind === SettingKind.bool) {
      if (entry.presentation === SettingPresentation.switch) {
        return (
          <Toggle
            kind="switch"
            id={id}
            label={label}
            labelHidden
            checked={value === "true"}
            disabled={pinned || disabledReason !== undefined}
            aria-describedby={describedBy}
            onCheckedChange={(checked) => onChange(String(checked))}
          />
        );
      }
      return (
        <Toggle
          id={id}
          label={label}
          labelHidden
          checked={value === "true"}
          disabled={pinned || disabledReason !== undefined}
          aria-describedby={describedBy}
          onCheckedChange={(checked) => onChange(String(checked))}
        />
      );
    }
    if (entry.kind === SettingKind.enum) {
      return (
        <Select value={value} disabled={pinned || disabledReason !== undefined} onValueChange={onChange}>
          <SelectTrigger
            id={id}
            className={pinnedSelect}
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
    if (entry.presentation === SettingPresentation.filler_download_schedule) {
      // The registry publishes presets in canonical form, so a resolved value matches by string.
      const presets = entry.presets ?? [];
      const matched = presets.find((option) => option.value === value);
      const preset = editingCustomSchedule || !matched ? CUSTOM_PRESET : matched.value;
      const current = durationParts(value);
      return (
        <div className="flex flex-col gap-2">
          <Select
            value={preset}
            disabled={pinned || disabledReason !== undefined}
            onValueChange={(next) => {
              if (next === CUSTOM_PRESET) {
                setEditingCustomSchedule(true);
                return;
              }
              setEditingCustomSchedule(false);
              onChange(next);
            }}
          >
            <SelectTrigger
              id={id}
              className={pinnedSelect}
              aria-describedby={describedBy}
              aria-labelledby={labelledBy}
              aria-invalid={invalid ? "true" : undefined}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {presets.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
              <SelectItem value={CUSTOM_PRESET}>Custom</SelectItem>
            </SelectContent>
          </Select>
          {preset === CUSTOM_PRESET && (
            <div className="flex gap-2">
              <NumberField
                accessibilityLabel="Custom check interval"
                amount={current.amount}
                disabled={disabledReason !== undefined}
                onAmount={(amount) => onChange(durationRaw(amount, current.unit))}
                readOnly={pinned}
              />
              <Select
                value={current.unit}
                disabled={pinned || disabledReason !== undefined}
                onValueChange={(unit) => onChange(durationRaw(current.amount, unit as DurationUnit))}
              >
                <SelectTrigger
                  aria-label="Custom check interval unit"
                  className={cn("w-32 shrink-0", pinnedSelect)}
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(["days", "hours", "minutes"] as const).map((unit) => (
                    <SelectItem key={unit} value={unit}>
                      {unit}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </div>
      );
    }
    if (entry.kind === SettingKind.duration) {
      const current = durationParts(value);
      return (
        <div className="flex gap-2">
          <NumberField
            amount={current.amount}
            aria-describedby={describedBy}
            aria-labelledby={labelledBy}
            disabled={disabledReason !== undefined}
            id={id}
            invalid={invalid}
            onAmount={(amount) => onChange(durationRaw(amount, current.unit))}
            readOnly={pinned}
          />
          <Select
            value={current.unit}
            disabled={pinned || disabledReason !== undefined}
            onValueChange={(unit) => onChange(durationRaw(current.amount, unit as DurationUnit))}
          >
            <SelectTrigger aria-label={`${label} unit`} className={cn("w-32 shrink-0", pinnedSelect)}>
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
    if (entry.presentation === SettingPresentation.bytes) {
      const current = byteParts(value);
      return (
        <WithUnit unit={current.unit}>
          <NumberField
            amount={current.amount}
            aria-describedby={describedBy}
            aria-labelledby={labelledBy}
            disabled={disabledReason !== undefined}
            id={id}
            invalid={invalid}
            onAmount={(amount) => onChange(String(Math.round(amount * 1024 ** 2)))}
            readOnly={pinned}
          />
        </WithUnit>
      );
    }
    const input = (
      <Surface backgroundColor="transparent" borderWidth={0} flexGrow={1} flexShrink={1} minWidth={0}>
        <Field
          aria-describedby={describedBy}
          aria-labelledby={labelledBy}
          autoCapitalize="none"
          autoComplete={entry.secret ? "new-password" : "off"}
          autoCorrect={false}
          disabled={disabledReason !== undefined}
          id={id}
          invalid={invalid}
          keyboardType={keyboardFor(entry.kind)}
          onChangeText={onChange}
          placeholder={entry.secret && entry.set ? "Enter a new value to replace" : undefined}
          readOnly={pinned}
          secureTextEntry={entry.secret}
          value={value}
        />
      </Surface>
    );
    // The registry declares what a plain number counts; "50" alone does not say GiB.
    return entry.unit ? <WithUnit unit={entry.unit}>{input}</WithUnit> : input;
  };

  // Compact: the control alone. The secret replace-flow is kept — a stored secret must never
  // render its value, in a table cell least of all.
  if (compact) {
    return (
      <div className={cn("min-w-0", className)}>
        {secretLocked ? (
          <span className="text-muted-foreground font-mono text-xs">{entry.preview ?? "stored"}</span>
        ) : (
          control()
        )}
      </div>
    );
  }

  return (
    // `group` so the audit line can reveal on hover/focus of the whole field (below).
    <div
      className={cn(
        "group",
        inlineSwitch
          ? "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1.5"
          : "flex flex-col gap-1.5",
        className,
      )}
    >
      <div className="flex items-center gap-2">
        <Label htmlFor={id}>{label}</Label>
        {/* The registry's one plain sentence sits under the field; the rest of the help (trade-offs,
            units, edge cases) waits behind this (i), so the form reads at a glance (#2741). */}
        {entry.detail && <FieldHelp label={label}>{entry.detail}</FieldHelp>}
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
              className="group/unlock focus-visible:outline-signal cursor-pointer rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2"
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
              {/* Both padlocks occupy ONE fixed box so the crossfade cannot reflow the chip's
                  text — a swap that changes width would make the label twitch on hover. The
                  hover/focus classes carry `!` because Tamagui's unlayered atomic CSS sets the
                  badge's background and colour and would otherwise beat these utilities. */}
              <Badge
                shape="square"
                icon={
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
                }
                className="group-hover/unlock:bg-signal-tint-15! group-hover/unlock:text-signal! group-focus-visible/unlock:bg-signal-tint-15! group-focus-visible/unlock:text-signal! transition-colors duration-200"
              >
                set via environment
              </Badge>
            </button>
          ) : (
            <Badge shape="square" icon={<Lock className="size-3" aria-hidden />}>
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
              <Badge shape="square" tone="caution" icon={<LockOpen className="size-3" aria-hidden />}>
                overriding {entry.envVar ?? "the environment"}
              </Badge>
            </button>
          ) : (
            <Badge shape="square" tone="caution" icon={<LockOpen className="size-3" aria-hidden />}>
              overriding {entry.envVar ?? "the environment"}
            </Badge>
          ))}
        {result?.status === SettingResultStatus.pinned && !pinned && (
          <Badge shape="square" tone="caution">
            not saved: pinned
          </Badge>
        )}
      </div>

      {secretLocked ? (
        <div className="flex items-center gap-2">
          <span className="border-input text-muted-foreground flex h-9 flex-1 items-center rounded-md border px-3 font-mono text-sm">
            {entry.preview ?? "stored"}
          </span>
          <button
            type="button"
            onClick={() => {
              setReplacing(true);
              onChange("");
            }}
            className="border-input hover:bg-accent cursor-pointer rounded-md border px-3 py-1.5 text-sm transition-colors"
          >
            Replace
          </button>
        </div>
      ) : (
        control()
      )}

      {/* The summary: one sentence, always visible, and the control's description. */}
      {entry.doc && (
        <FieldHint id={`${id}-doc`} className={cn(inlineSwitch && "col-span-2")}>
          {entry.doc}
        </FieldHint>
      )}

      {entry.caution && (
        <FieldHint tone="error" className={cn("flex items-center gap-1", inlineSwitch && "col-span-2")}>
          <TriangleAlert className="size-3" aria-hidden />
          The stored value was invalid and has been reset to the default.
        </FieldHint>
      )}

      {disabledReason && (
        <FieldHint
          id={`${id}-unavailable`}
          role="alert"
          tone="error"
          className={cn("flex items-start gap-1", inlineSwitch && "col-span-2")}
        >
          <TriangleAlert className="mt-0.5 size-3 shrink-0" aria-hidden />
          {disabledReason}
        </FieldHint>
      )}

      {/* "changed by … · when" (§5 field anatomy). Audit provenance is signal when you're
          auditing, noise at rest — so it's revealed on hover/focus of the field rather than
          costing a permanent line under every control (config-design §5). It stays in the DOM
          (opacity, not conditional render) so it's reachable by keyboard (focus-within) and
          screen readers; the transition is frozen under reduced-motion. Only for a value a
          PERSON set: an env pin or built-in default has no author to name. */}
      {entry.updatedAt && (
        <p
          className={cn(
            "text-muted-foreground pointer-events-none text-xs opacity-0 transition-opacity duration-150 group-focus-within:opacity-100 group-hover:opacity-100 motion-reduce:transition-none",
            inlineSwitch && "col-span-2",
          )}
        >
          {entry.updatedBy ? `Changed by ${entry.updatedBy} · ` : "Changed "}
          {formatRelative(entry.updatedAt)}
        </p>
      )}

      {problem && (
        <FieldHint
          id={`${id}-problem`}
          role="alert"
          tone="error"
          className={cn(inlineSwitch && "col-span-2")}
        >
          {problem}
        </FieldHint>
      )}
    </div>
  );
};

export { SettingField };
