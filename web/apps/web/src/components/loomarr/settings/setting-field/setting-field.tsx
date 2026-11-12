import { SettingEntryProvenance } from "@loomarr/api/models/settingEntryProvenance";
import { SettingKind } from "@loomarr/api/models/settingKind";
import { SettingPresentation } from "@loomarr/api/models/settingPresentation";
import type { SettingPreset } from "@loomarr/api/models/settingPreset";
import { SettingResultStatus } from "@loomarr/api/models/settingResultStatus";
import { SettingUnit } from "@loomarr/api/models/settingUnit";
import { formatAgo, humanizeSettingKey } from "@loomarr/core/format";
import {
  Action,
  CompactSelect,
  Field,
  FieldHint,
  Icon,
  icons,
  Stack,
  StatusLabel,
  Surface,
  Text,
  Toggle,
} from "@loomarr/design-system";
import { NumberField } from "@loomarr/ui";
import { type ComponentProps, type ReactNode, useState } from "react";
import { languageName } from "@/lib/languages";
import { cn } from "@/lib/utils";
import { FieldHelp } from "../../feedback";
import { CronField } from "./cron-field";
import { DurationField, type DurationUnit } from "./duration-field";
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

// A Go duration string ("6h0m0s", "90s", "1h30m") as seconds; anything unreadable is zero.
const durationSeconds = (raw: string): number => {
  const factors: Record<string, number> = { h: 3600, m: 60, s: 1, ms: 0.001 };
  let seconds = 0;
  for (const match of raw.matchAll(/(-?\d+(?:\.\d+)?)(ms|h|m|s)/g)) {
    const factor = factors[match[2] ?? ""];
    if (factor !== undefined) seconds += Number(match[1]) * factor;
  }
  return Number.isFinite(seconds) ? seconds : 0;
};

const durationRaw = (seconds: number): string => `${Number(seconds.toFixed(3))}s`;

// A whole number of time, per its registry-declared unit: what one stored step is worth in seconds
// and the units it can be edited in. Only this unit and larger ones, so every edit stays whole.
const TIME_UNITS: Partial<Record<SettingUnit, { seconds: number; units: readonly DurationUnit[] }>> = {
  [SettingUnit.seconds]: { seconds: 1, units: ["days", "hours", "minutes", "seconds"] },
  [SettingUnit.hours]: { seconds: 3600, units: ["days", "hours"] },
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
  <Stack direction="row" gap="inline">
    {children}
    <span className="border-input text-muted-foreground flex h-10 w-16 shrink-0 items-center justify-center rounded-md border text-sm">
      {unit}
    </span>
  </Stack>
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
  // disabled look that reads as an empty placeholder (#2759): every control takes `readOnly`.
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
        <CompactSelect
          aria-describedby={describedBy}
          aria-labelledby={labelledBy}
          disabled={disabledReason !== undefined}
          id={id}
          invalid={invalid}
          label={label}
          onValueChange={onChange}
          // Prefer the registry-owned {value,label} options (config-design §5) so the dropdown
          // shows "OpenAI"/"Emby", not the raw stored value. Fall back to the bare value list for
          // any enum that ships no labels.
          options={entry.enumOptions ?? (entry.enum ?? []).map((v) => ({ value: v, label: v }))}
          readOnly={pinned}
          value={value}
        />
      );
    }
    if (entry.presentation === SettingPresentation.filler_download_schedule) {
      // The registry publishes presets in canonical form, so a resolved value matches by string.
      const presets = entry.presets ?? [];
      const matched = presets.find((option) => option.value === value);
      const preset = editingCustomSchedule || !matched ? CUSTOM_PRESET : matched.value;
      return (
        <Stack gap="inline">
          <CompactSelect
            aria-describedby={describedBy}
            aria-labelledby={labelledBy}
            disabled={disabledReason !== undefined}
            id={id}
            invalid={invalid}
            label={label}
            onValueChange={(next) => {
              if (next === CUSTOM_PRESET) {
                setEditingCustomSchedule(true);
                return;
              }
              setEditingCustomSchedule(false);
              onChange(next);
            }}
            options={[...presets, { label: "Custom", value: CUSTOM_PRESET }]}
            readOnly={pinned}
            value={preset}
          />
          {preset === CUSTOM_PRESET && (
            <DurationField
              amountLabel="Custom check interval"
              disabled={pinned || disabledReason !== undefined}
              label="Custom check interval"
              onSeconds={(seconds) => onChange(durationRaw(seconds))}
              seconds={durationSeconds(value)}
              units={["days", "hours", "minutes"]}
            />
          )}
        </Stack>
      );
    }
    if (entry.kind === SettingKind.cron) {
      return (
        <CronField
          id={id}
          label={label}
          value={value}
          presets={entry.presets ?? []}
          onChange={onChange}
          disabled={disabledReason !== undefined}
          readOnly={pinned}
          invalid={invalid}
          describedBy={describedBy}
          labelledBy={labelledBy}
        />
      );
    }
    if (entry.kind === SettingKind.duration) {
      return (
        <DurationField
          describedBy={describedBy}
          disabled={pinned || disabledReason !== undefined}
          id={id}
          invalid={invalid}
          label={label}
          labelledBy={labelledBy}
          onSeconds={(seconds) => onChange(durationRaw(seconds))}
          seconds={durationSeconds(value)}
          units={["days", "hours", "minutes", "seconds"]}
        />
      );
    }
    const timeUnit = entry.kind === SettingKind.int && entry.unit ? TIME_UNITS[entry.unit] : undefined;
    if (timeUnit) {
      const stored = Number(value);
      return (
        <DurationField
          describedBy={describedBy}
          disabled={pinned || disabledReason !== undefined}
          id={id}
          invalid={invalid}
          label={label}
          labelledBy={labelledBy}
          onSeconds={(seconds) => onChange(String(Math.round(seconds / timeUnit.seconds)))}
          seconds={(Number.isFinite(stored) ? stored : 0) * timeUnit.seconds}
          units={timeUnit.units}
        />
      );
    }
    if (
      entry.kind === SettingKind.string &&
      (entry.presentation === SettingPresentation.timezone ||
        entry.presentation === SettingPresentation.language)
    ) {
      // The server's choices, never free text. A pinned or stored value outside them (a legacy zone
      // name, a rare language) is still shown as chosen rather than silently swapped for another.
      const presets = entry.presets ?? [];
      const choices = presets.some((option) => option.value === value)
        ? presets
        : [...presets, { value, label: value }];
      // Language names are interface copy, localized from CLDR; the registry's labels are codes.
      const optionLabel = (option: SettingPreset) =>
        entry.presentation === SettingPresentation.language && option.value
          ? languageName(option.value)
          : option.label;
      return (
        <CompactSelect
          aria-describedby={describedBy}
          aria-labelledby={labelledBy}
          disabled={pinned || disabledReason !== undefined}
          id={id}
          invalid={invalid}
          label={label}
          onValueChange={onChange}
          options={choices.map((option) => ({ value: option.value, label: optionLabel(option) }))}
          value={value}
        />
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
        // `relative` anchors the absolutely positioned `sr-only` doc below to this field.
        "group relative",
        inlineSwitch
          ? "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1.5"
          : "flex flex-col gap-1.5",
        className,
      )}
    >
      <Stack align="center" direction="row" gap="inline">
        <Text as="label" htmlFor={id} textRole="label">
          {label}
        </Text>
        {/* The registry's one plain sentence sits under the field; the rest of the help (trade-offs,
            units, edge cases) waits behind this (i), so the form reads at a glance (#2741). */}
        {entry.detail && <FieldHelp label={label}>{entry.detail}</FieldHelp>}
        {/* THE LOCK IS THE CONTROL (config-design §3.1). The chip already says why the field
            is read-only, so hanging a separate "unlock" button beside it would put the
            explanation and the way out in two places — the operator reads "set via
            environment", and the thing to click is somewhere else. Clicking the lock opens
            it; clicking the open lock hands the key back.

            Still a plain label when no handler is supplied: a read-only surface keeps the
            pre-3.1 state exactly, rather than rendering a button that does nothing. */}
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
              {/* Both padlocks occupy ONE fixed box so the crossfade cannot reflow the label's
                  text — a swap that changes width would make the label twitch on hover. The
                  padlock is the control and keeps its open-on-hover tell; the state itself is
                  the quiet dot and word, not a filled chip (#2784). */}
              <span className="text-muted-foreground group-hover/unlock:text-signal group-focus-visible/unlock:text-signal flex items-center gap-1.5">
                <span className="relative inline-flex size-4 shrink-0 items-center justify-center">
                  <Stack
                    className="absolute transition-opacity duration-(--duration-fast) group-hover/unlock:opacity-0 group-focus-visible/unlock:opacity-0"
                    direction="row"
                    render="span"
                  >
                    <Icon decorative glyph={icons.locked} size="compact" tone="inherit" />
                  </Stack>
                  <Stack
                    className="absolute opacity-0 transition-opacity duration-(--duration-fast) group-hover/unlock:opacity-100 group-focus-visible/unlock:opacity-100"
                    direction="row"
                    render="span"
                  >
                    <Icon decorative glyph={icons.unlocked} size="compact" tone="inherit" />
                  </Stack>
                </span>
                <StatusLabel tone="off">Set via environment</StatusLabel>
              </span>
            </button>
          ) : (
            <StatusLabel tone="off">Set via environment</StatusLabel>
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
              <StatusLabel tone="warn">Overriding {entry.envVar ?? "the environment"}</StatusLabel>
            </button>
          ) : (
            <StatusLabel tone="warn">Overriding {entry.envVar ?? "the environment"}</StatusLabel>
          ))}
        {result?.status === SettingResultStatus.pinned && !pinned && (
          <StatusLabel tone="warn">Not saved: pinned</StatusLabel>
        )}
      </Stack>

      {secretLocked ? (
        <Stack align="center" direction="row" gap="inline">
          <span className="border-input text-muted-foreground flex h-9 flex-1 items-center rounded-md border px-3 font-mono text-sm">
            {entry.preview ?? "stored"}
          </span>
          <Action
            onPress={() => {
              setReplacing(true);
              onChange("");
            }}
            variant="outline"
          >
            Replace
          </Action>
        </Stack>
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
          <Icon decorative glyph={icons.alert} size="compact" tone="inherit" />
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
          <Stack className="mt-0.5" direction="row" render="span">
            <Icon decorative glyph={icons.alert} size="compact" tone="inherit" />
          </Stack>
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
          {/* The server sends a name (never a raw id) and marks the caller's own change (#2736). */}
          {entry.updatedByYou
            ? `Changed by you, ${formatAgo(entry.updatedAt)}`
            : entry.updatedBy
              ? `Changed by ${entry.updatedBy}, ${formatAgo(entry.updatedAt)}`
              : `Changed ${formatAgo(entry.updatedAt)}`}
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
