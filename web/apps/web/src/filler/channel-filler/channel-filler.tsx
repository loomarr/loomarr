import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import type { CoverageDTO } from "@loomarr/api/models/coverageDTO";
import type { PodEntryDTO } from "@loomarr/api/models/podEntryDTO";
import { unwrap } from "@loomarr/api/unwrap";
import { pluralize } from "@loomarr/core/format";
import { Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { useEffect, useState } from "react";
import { CollapsibleSection } from "@/components/loomarr/feedback/collapsible-section";
import { ClipPreview } from "@/components/loomarr/filler/clip-preview";
import { CoverageMeter } from "@/components/loomarr/filler/coverage-meter";
import { PodTimeline } from "@/components/loomarr/filler/pod-timeline";
import { usePlaceLabel } from "@/components/loomarr/settings/installation-location";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { eraOf } from "@/lib/era-dates";
import { cn } from "@/lib/utils";
import { useChannelFillerDraft } from "../use-channel-filler-draft";
import type { ChannelFillerProps } from "./channel-filler.type";
import { FillerClipList } from "./filler-clip-list";
import { FillerCriteria } from "./filler-criteria";
import { useFillerCatalog } from "./use-filler-catalog";

type BreakFrequencyFieldProps = {
  value: number | undefined;
  defaultValue: number | undefined;
  disabled: boolean;
  onChange: (next: number | undefined) => void;
};

type DurationChoiceFieldProps = {
  // A Go duration string ("2m0s", "4h"); undefined follows the default.
  value: string | undefined;
  defaultValue: string | undefined;
  disabled: boolean;
  onChange: (next: string | undefined) => void;
};

// Go writes durations with every unit ("4h0m0s", "1h30m0s"); the field also accepts the short
// forms it sends itself ("2m", "4h"). Returns total minutes.
const durationMinutes = (value: string | undefined): number | undefined => {
  const text = value?.trim() ?? "";
  if (!/^(?:[0-9]+(?:\.[0-9]+)?(?:ms|s|m|h))+$/.test(text)) return undefined;
  const perMinute: Record<string, number> = { ms: 1 / 60_000, s: 1 / 60, m: 1, h: 60 };
  let minutes = 0;
  for (const [, amount, unit] of text.matchAll(/([0-9]+(?:\.[0-9]+)?)(ms|s|m|h)/g)) {
    minutes += Number(amount) * (perMinute[unit ?? "m"] ?? 1);
  }
  return minutes;
};

type DurationChoice = {
  id: string;
  label: string;
  customOption: string;
  customAriaLabel: string;
  // The unit the custom number is typed in, and its Go duration suffix.
  unit: { suffix: "m" | "h"; minutes: number; words: string; describe: (n: number) => string };
  min: number;
  step: number;
  // Shown when the default setting cannot be read.
  fallbackDefault: number;
  help: string;
};

const BREAK_LENGTH: DurationChoice = {
  id: "channel-break-duration",
  label: "Break length",
  customOption: "Custom length",
  customAriaLabel: "Custom break length in minutes",
  unit: { suffix: "m", minutes: 1, words: "minutes", describe: (n) => `${n} min` },
  min: 0.5,
  step: 0.5,
  fallbackDefault: 2,
  help: "Loomarr automatically uses enough clips to fill this target. The minimum is 30 seconds.",
};

const COMMERCIAL_SPACING: DurationChoice = {
  id: "channel-commercial-spacing",
  label: "Repeat spacing · same commercial",
  customOption: "Custom spacing",
  customAriaLabel: "Custom commercial repeat spacing in hours",
  unit: { suffix: "h", minutes: 60, words: "hours", describe: (n) => pluralize(n, "hour") },
  min: 0,
  step: 0.5,
  fallbackDefault: 4,
  help: "Minimum time before a commercial you saw on any channel airs again here. Loomarr first uses commercials from a nearby year, and repeats one sooner only when a break would otherwise run short.",
};

// One field for a channel duration that follows a default or overrides it. Break length uses it
// with deliberately no "off" sentinel: frequency owns whether a channel has breaks, keeping a
// zero duration from meaning two different things. Commercial spacing allows 0 (repeat freely).
const DurationChoiceField = ({
  choice,
  value,
  defaultValue,
  disabled,
  onChange,
}: DurationChoiceFieldProps & { choice: DurationChoice }) => {
  const { unit, min } = choice;
  const inUnit = (duration: string | undefined) => {
    const minutes = durationMinutes(duration);
    return minutes === undefined ? undefined : minutes / unit.minutes;
  };
  const mode = value === undefined ? "default" : "custom";
  const defaultAmount = inUnit(defaultValue) ?? choice.fallbackDefault;
  const [customValue, setCustomValue] = useState(String(inUnit(value) ?? defaultAmount));

  useEffect(() => {
    const minutes = durationMinutes(value);
    if (minutes !== undefined) setCustomValue(String(minutes / unit.minutes));
  }, [value, unit.minutes]);

  const options = [
    { value: "default", label: `Follow default (${unit.describe(defaultAmount)})` },
    { value: "custom", label: choice.customOption },
  ];

  const changeMode = (next: string) => {
    if (next === "default") onChange(undefined);
    if (next === "custom") {
      const parsed = Number(customValue);
      const nextValue = Number.isFinite(parsed) && parsed >= min ? parsed : defaultAmount;
      setCustomValue(String(nextValue));
      onChange(`${nextValue}${unit.suffix}`);
    }
  };

  const changeCustomValue = (next: string) => {
    setCustomValue(next);
    const parsed = Number(next);
    if (next.trim() !== "" && Number.isFinite(parsed) && parsed >= min) onChange(`${parsed}${unit.suffix}`);
  };

  const restoreValidValue = () => {
    const parsed = Number(customValue);
    if (customValue.trim() === "" || !Number.isFinite(parsed) || parsed < min) {
      setCustomValue(String(inUnit(value) ?? defaultAmount));
    }
  };

  return (
    <div className="grid gap-2 sm:max-w-md">
      <Label htmlFor={choice.id}>{choice.label}</Label>
      <Select
        key={options[0]?.label}
        value={mode}
        items={options}
        onValueChange={changeMode}
        disabled={disabled}
      >
        <SelectTrigger id={choice.id}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {mode === "custom" && (
        <div className="flex items-center gap-2">
          <Input
            aria-label={choice.customAriaLabel}
            className="w-24"
            type="number"
            min={min}
            step={choice.step}
            inputMode="decimal"
            value={customValue}
            disabled={disabled}
            onChange={(event) => changeCustomValue(event.target.value)}
            onBlur={restoreValidValue}
          />
          <span className="text-muted-foreground text-sm">{unit.words}</span>
        </div>
      )}
      <p className="text-muted-foreground text-sm">{choice.help}</p>
    </div>
  );
};

// The one-line summary of what this channel's pool is limited to, from the coverage the backend
// computed: its location boundary and how many commercials fit. Undefined until coverage loads.
const poolSummary = (
  coverage: CoverageDTO | undefined,
  placeLabel: string | undefined,
): string | undefined => {
  if (!coverage) return undefined;
  const { country } = coverage.geography;
  const fits = `${coverage.total} ${coverage.total === 1 ? "fits" : "fit"} this channel`;
  if (!country) return fits;
  return `${placeLabel ?? country} commercials only · ${fits}`;
};

// The persisted pointer has three meaningful states. This field owns that encoding so the
// surrounding page can speak in operator choices rather than nil/zero sentinel values.
const BreakFrequencyField = ({ value, defaultValue, disabled, onChange }: BreakFrequencyFieldProps) => {
  const mode = value === undefined ? "default" : value === 0 ? "off" : "custom";
  const seed = defaultValue != null && defaultValue > 0 ? defaultValue : 1;
  const [customValue, setCustomValue] = useState(String(value != null && value > 0 ? value : seed));

  useEffect(() => {
    if (value != null && value > 0) setCustomValue(String(value));
  }, [value]);

  const defaultLabel =
    defaultValue === 0
      ? "Follow default (no breaks)"
      : defaultValue != null
        ? `Follow default (${defaultValue} per hour)`
        : "Follow default";
  const options = [
    { value: "default", label: defaultLabel },
    { value: "off", label: "No commercial breaks" },
    { value: "custom", label: "Custom frequency" },
  ];

  const changeMode = (next: string) => {
    if (next === "default") onChange(undefined);
    if (next === "off") onChange(0);
    if (next === "custom") {
      const remembered = Number(customValue);
      const nextValue = Number.isInteger(remembered) && remembered >= 1 ? remembered : seed;
      setCustomValue(String(nextValue));
      onChange(nextValue);
    }
  };

  const changeCustomValue = (next: string) => {
    setCustomValue(next);
    const parsed = Number(next);
    if (Number.isInteger(parsed) && parsed >= 1) onChange(parsed);
  };

  const restoreValidValue = () => {
    const parsed = Number(customValue);
    if (!Number.isInteger(parsed) || parsed < 1) setCustomValue(String(value && value > 0 ? value : seed));
  };

  return (
    <div className="grid gap-2 sm:max-w-md">
      <Label htmlFor="channel-break-frequency">Break frequency</Label>
      <Select key={defaultLabel} value={mode} items={options} onValueChange={changeMode} disabled={disabled}>
        <SelectTrigger id="channel-break-frequency">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {mode === "custom" && (
        <div className="flex items-center gap-2">
          <Input
            aria-label="Custom breaks per program hour"
            className="w-24"
            type="number"
            min={1}
            step={1}
            inputMode="numeric"
            value={customValue}
            disabled={disabled}
            onChange={(event) => changeCustomValue(event.target.value)}
            onBlur={restoreValidValue}
          />
          <span className="text-muted-foreground text-sm">breaks per program hour</span>
        </div>
      )}
      <p className="text-muted-foreground text-sm">
        How often this channel pauses for filler. Manage the inherited value in{" "}
        <Link to="/settings/defaults" className="text-signal underline-offset-2 hover:underline">
          Channel defaults
        </Link>
        .
      </p>
    </div>
  );
};

// ChannelFiller — the per-channel filler sandbox (§10, §12), the Filler section on the
// channel page. Three parts over one live draft: the THEME criteria (era/audience/
// categories/kinds), the OVERRIDES (pin "always include" / exclude "never include"), and a
// live PREVIEW of the actual break the draft would air. Editing anything re-assembles the
// break in front of you through the SAME assembler reconcile uses — nothing saved until
// Apply, which commits the draft to policy.filler and lets reconcile take over (seamless
// for the effect; draft/apply for the authoring, the one deliberate §10 exception).
const ChannelFiller = ({ channelId, policy, from, className }: ChannelFillerProps) => {
  const [previewingClip, setPreviewingClip] = useState<PodEntryDTO>();
  const {
    draft,
    setDraft,
    breaksPerHour,
    setBreaksPerHour,
    breakDuration,
    setBreakDuration,
    commercialSpacing,
    setCommercialSpacing,
    preview,
    isPreviewing,
    previewError,
    isDirty,
    apply,
    isApplying,
    discard,
  } = useChannelFillerDraft(channelId, policy);

  const pinned = draft.pinned ?? [];
  const excluded = draft.excluded ?? [];

  // A small shared lookup so pinned/excluded ids can render as names, and so pin can't offer a
  // clip already excluded (and vice-versa).
  //
  // ⚠ It asks for exactly THESE ids (§10 V51d), not the catalog. Reading the catalog and mapping
  // it client-side worked only while the listing was unbounded; against a 100-row page it would
  // resolve whichever pins happened to land on page one and render the rest as bare hashes.
  const { resolve, isLoading: resolving } = useFillerCatalog([...pinned, ...excluded]);

  // Global inherited break defaults. Read from the settings list rather than added to a channel
  // DTO: they are global knobs, and the section is admin-only.
  // ⚠ `retry: false` for the same reason the coverage query uses it — a filler-less install
  // answers 501 and three retries is console noise for a state that simply renders nothing.
  const settings = settingsApi.useSettingsList({ query: { retry: false } });
  const settingEntries = unwrap(settings.data, (b) => b.settings) ?? [];
  const defaultBreaksEntry = settingEntries.find((e) => e.key === "filler.breaks_per_hour");
  const defaultBreaksPerHour = defaultBreaksEntry ? Number(defaultBreaksEntry.value) : undefined;
  const defaultBreakDuration = settingEntries.find((e) => e.key === "filler.break_duration")?.value;
  // A whole number of seconds; the field reads durations, so it gets the seconds unit.
  const defaultSpacingSeconds = settingEntries.find((e) => e.key === "filler.cooldown_seconds")?.value;
  const defaultCommercialSpacing = defaultSpacingSeconds ? `${defaultSpacingSeconds}s` : undefined;
  const homeCountry = settingEntries.find((e) => e.key === "filler.home_country")?.value;
  const homePlace = settingEntries.find((e) => e.key === "filler.home_place")?.value;

  // Coverage for the channel's SAVED selection (V29b). A filler-less install answers 501;
  // retrying that three times is noise for a diagnostic disclosure that simply renders nothing.
  const coverageQuery = channelsApi.useChannelFillerCoverage(channelId, {
    query: { retry: false },
  });
  const coverage = unwrap(coverageQuery.data);
  const poolPlaceLabel = usePlaceLabel(coverage?.geography.place);

  const entries = preview?.entries ?? [];

  return (
    <div className={cn("flex flex-col gap-6", className)}>
      <div>
        {/* Reached from a Filler Overview coverage card, this page is a detour out of the Filler
            area; the header's back arrow goes to the channel list, so the detour gets its own way
            home (#1817, decided on #1908). The Settings index's "← Manage" link is the pattern. */}
        {from === "filler" ? (
          <Link
            to="/filler"
            className="text-muted-foreground hover:text-foreground mb-3 inline-flex items-center gap-1.5 text-sm"
          >
            <ArrowLeft className="size-3.5" aria-hidden />
            Filler overview
          </Link>
        ) : null}
        <h2 className="text-lg font-semibold">Filler</h2>
        <p className="text-muted-foreground text-sm">
          This channel picks its commercials and clips automatically. Only change these settings if it needs
          something different.
        </p>
      </div>

      {/* Saved truth comes before authoring. An operator should see whether this channel already
          works before meeting the controls that can make its pool narrower. This is intentionally
          not collapsed: coverage is the primary answer, not advanced diagnosis. The headings and
          sentences on this page are the web mock's. */}
      <section className="border-border bg-card flex flex-col gap-3 rounded-lg border p-5">
        <div>
          <h3 className="text-base font-semibold">What this channel can play</h3>
          <p className="text-muted-foreground text-sm">
            {poolSummary(coverage, poolPlaceLabel) ?? "Clips that fit this channel's settings right now."}
          </p>
        </div>
        {coverageQuery.isLoading ? (
          <p className="text-muted-foreground text-sm">Checking saved coverage…</p>
        ) : coverageQuery.error ? (
          <p className="text-onair-300 text-sm">
            Saved coverage could not be checked. Your existing channel settings are unchanged.
          </p>
        ) : coverage ? (
          <CoverageMeter coverage={coverage} />
        ) : (
          <p className="text-muted-foreground text-sm">
            Coverage becomes available after filler is configured for this install.
          </p>
        )}
      </section>

      <section className="border-border bg-card flex flex-col gap-5 rounded-lg border p-5">
        <div>
          <h3 className="text-base font-semibold">Choose what fits</h3>
          <p className="text-muted-foreground text-sm">
            Start broad. Add only the filters this channel really needs. Clips come from your{" "}
            <Link to="/filler/library" className="text-signal underline-offset-2 hover:underline">
              filler library
            </Link>
            .
          </p>
        </div>

        <BreakFrequencyField
          value={breaksPerHour}
          defaultValue={defaultBreaksPerHour}
          disabled={isApplying}
          onChange={setBreaksPerHour}
        />

        <DurationChoiceField
          choice={BREAK_LENGTH}
          value={breakDuration}
          defaultValue={defaultBreakDuration}
          disabled={isApplying}
          onChange={setBreakDuration}
        />

        <DurationChoiceField
          choice={COMMERCIAL_SPACING}
          value={commercialSpacing}
          defaultValue={defaultCommercialSpacing}
          disabled={isApplying}
          onChange={setCommercialSpacing}
        />

        {/* scopeEra so the criteria panel can SHOW that a blank era follows the channel's own
            (§10 V51f) — the server has always applied it, and nothing said so. */}
        <FillerCriteria
          selection={draft}
          onChange={setDraft}
          disabled={isApplying}
          scopeEra={eraOf(policy?.scope?.dates)}
          programmingDates={policy?.scope?.dates}
          installationGeography={{ country: homeCountry, place: homePlace }}
        />
      </section>

      <CollapsibleSection
        size="compact"
        title="Specific clips (advanced)"
        description={`${
          pinned.length + excluded.length === 0
            ? "None set."
            : `${pluralize(pinned.length + excluded.length, "saved override")}.`
        } Always include, or never include, particular clips.`}
      >
        <div className="flex flex-col gap-4">
          <FillerClipList
            label="Prefer on this channel"
            hint="Play these clips ahead of automatic rotation, even when year, audience, or topics differ. Location, type, and length restrictions still apply. Preferred clips may repeat."
            ids={pinned}
            onChange={(next) => setDraft({ ...draft, pinned: next })}
            resolve={resolve}
            resolving={resolving}
            disabled={isApplying}
            excludeIds={excluded}
          />
          <FillerClipList
            label="Exclude from this channel"
            hint="Never play these clips on this channel. Exclusion wins over a preference."
            ids={excluded}
            onChange={(next) => setDraft({ ...draft, excluded: next })}
            resolve={resolve}
            resolving={resolving}
            disabled={isApplying}
            excludeIds={pinned}
          />
        </div>
      </CollapsibleSection>

      {/* The live break — the ground-truth preview of the current draft. */}
      <section className="border-border bg-card flex flex-col gap-2 rounded-lg border p-5">
        <h3 className="text-base font-semibold">Preview break</h3>
        {previewError ? (
          <p className="text-onair-300 text-sm">
            Couldn't assemble a preview for this selection. Adjust the criteria and try again.
          </p>
        ) : entries.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {isPreviewing ? (
              "Assembling preview…"
            ) : (
              <>
                No clips match these choices yet. Breaks use the bumper card for now. Adjust the choices
                above, or{" "}
                <Link to="/filler/library" className="text-signal underline-offset-2 hover:underline">
                  browse your filler library
                </Link>
                .
              </>
            )}
          </p>
        ) : (
          <div className={cn("transition-opacity", isPreviewing && "opacity-60")}>
            <PodTimeline entries={entries} matchLevel={preview?.matchLevel} onPreview={setPreviewingClip} />
            <p className="text-muted-foreground mt-2 text-sm">
              {`A sample break of ${pluralize(entries.length, "clip")}, put together the way this channel does it. Click a clip to play it.`}
            </p>
          </div>
        )}
      </section>

      {/* Apply / Discard — only offered when the draft differs from what's saved. Apply is
          the seamless commit; Discard throws the draft away. */}
      {isDirty && (
        <div className="border-border bg-background/95 sticky bottom-0 flex items-center gap-2 rounded-lg border p-3 shadow-lg backdrop-blur">
          <Button variant="suggest" size="sm" disabled={isApplying} onClick={apply}>
            {isApplying ? "Applying…" : "Apply filler"}
          </Button>
          <Button variant="ghost" size="sm" disabled={isApplying} onClick={discard}>
            Discard
          </Button>
          <span className="text-muted-foreground ml-auto text-xs">Unsaved changes</span>
        </div>
      )}

      <Sheet open={Boolean(previewingClip)} onOpenChange={(open) => !open && setPreviewingClip(undefined)}>
        {previewingClip?.hash ? (
          <SheetContent>
            <SheetHeader>
              <SheetTitle>{previewingClip.name}</SheetTitle>
              <SheetDescription>Preview from this assembled break</SheetDescription>
            </SheetHeader>
            <div className="p-6">
              <div className="border-border overflow-hidden rounded-lg border">
                <ClipPreview clip={{ hash: previewingClip.hash, name: previewingClip.name }} />
              </div>
            </div>
          </SheetContent>
        ) : null}
      </Sheet>
    </div>
  );
};

export { ChannelFiller };
