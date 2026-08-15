import { channelsApi, settingsApi, unwrap } from "@loomarr/api";
import { pluralize } from "@loomarr/core";
import { Link } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { CollapsibleSection, CoverageMeter, PodTimeline } from "@/components/loomarr";
import {
  Button,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui";
import { cn } from "@/lib";
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
const ChannelFiller = ({ channelId, revision, policy, className }: ChannelFillerProps) => {
  const {
    draft,
    setDraft,
    breaksPerHour,
    setBreaksPerHour,
    preview,
    isPreviewing,
    previewError,
    isDirty,
    apply,
    isApplying,
    discard,
  } = useChannelFillerDraft(channelId, policy, revision);

  const pinned = draft.pinned ?? [];
  const excluded = draft.excluded ?? [];

  // A small shared lookup so pinned/excluded ids can render as names, and so pin can't offer a
  // clip already excluded (and vice-versa).
  //
  // ⚠ It asks for exactly THESE ids (§10 V51d), not the catalog. Reading the catalog and mapping
  // it client-side worked only while the listing was unbounded; against a 100-row page it would
  // resolve whichever pins happened to land on page one and render the rest as bare hashes.
  const { resolve, isLoading: resolving } = useFillerCatalog([...pinned, ...excluded]);

  // `filler.pod_max` — the cap on clips per break, so the pin list can say when it exceeds it
  // (#237). Read from the settings list the sibling filler panels already use rather than added
  // to a DTO: it is a global knob, not a property of this channel, and the section is admin-only.
  // ⚠ `retry: false` for the same reason the coverage query uses it — a filler-less install
  // answers 501 and three retries is console noise for a state that simply renders nothing.
  const settings = settingsApi.useSettingsList({ query: { retry: false } });
  const settingEntries = unwrap(settings.data, (b) => b.settings) ?? [];
  const podMaxEntry = settingEntries.find((e) => e.key === "filler.pod_max");
  const podMax = podMaxEntry ? Number(podMaxEntry.value) : undefined;
  const defaultBreaksEntry = settingEntries.find((e) => e.key === "filler.breaks_per_hour");
  const defaultBreaksPerHour = defaultBreaksEntry ? Number(defaultBreaksEntry.value) : undefined;

  // Coverage for the channel's SAVED selection (V29b). A filler-less install answers 501;
  // retrying that three times is noise for a diagnostic disclosure that simply renders nothing.
  const coverageQuery = channelsApi.useChannelFillerCoverage(channelId, {
    query: { retry: false },
  });
  const coverage = unwrap(coverageQuery.data);

  const entries = preview?.entries ?? [];

  return (
    <div className={cn("flex flex-col gap-6", className)}>
      <div>
        <h2 className="font-semibold text-lg">Filler</h2>
        <p className="text-muted-foreground text-sm">
          Choose what plays between shows, then preview the actual break before applying it.
        </p>
      </div>

      <section className="flex flex-col gap-5 rounded-lg border border-border p-5">
        <div>
          <h3 className="font-semibold text-base">Match this channel</h3>
          <p className="text-muted-foreground text-sm">
            Start broad. Add only the filters this channel really needs. Clips come from your{" "}
            <Link to="/filler" className="text-signal underline-offset-2 hover:underline">
              filler catalog
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

        {/* scopeEra so the criteria panel can SHOW that a blank era follows the channel's own
            (§10 V51f) — the server has always applied it, and nothing said so. */}
        <FillerCriteria
          selection={draft}
          onChange={setDraft}
          disabled={isApplying}
          scopeEra={policy?.scope?.era ?? undefined}
        />
      </section>

      <CollapsibleSection
        title="Specific clips"
        description="Optionally pin clips to every break or block clips from this channel."
        defaultOpen={pinned.length > 0 || excluded.length > 0}
      >
        <div className="flex flex-col gap-4">
          <FillerClipList
            label="Always include"
            hint="Pin specific clips so every break can use them, ahead of the theme."
            ids={pinned}
            onChange={(next) => setDraft({ ...draft, pinned: next })}
            resolve={resolve}
            resolving={resolving}
            // Only the PIN list is capped by pod_max — an exclusion list of any length costs
            // nothing, because excluding is a filter rather than a thing to fit in a break.
            cap={podMax}
            disabled={isApplying}
            excludeIds={excluded}
          />
          <FillerClipList
            label="Never include"
            hint="Block clips you don't want on this channel. They win over a pin."
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
      <section className="flex flex-col gap-2 rounded-lg border border-border p-5">
        <h3 className="font-semibold text-base">Preview break</h3>
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
                No clips match this selection yet. Breaks fall back to the bumper card. Loosen the theme
                above, or{" "}
                <Link to="/filler" className="text-signal underline-offset-2 hover:underline">
                  add and tag clips
                </Link>{" "}
                in your filler catalog.
              </>
            )}
          </p>
        ) : (
          <div className={cn("transition-opacity", isPreviewing && "opacity-60")}>
            <PodTimeline entries={entries} matchLevel={preview?.matchLevel} />
            <p className="mt-2 text-muted-foreground text-sm">
              {`${pluralize(entries.length, "clip")} in this break, assembled exactly as the channel builds it.`}
            </p>
          </div>
        )}
      </section>

      {/* Why the saved break looks the way it does (V29b). It is diagnosis, so it stays quiet
          until an operator needs to understand a fallback or an unexpectedly small pool. */}
      {coverage && (
        <CollapsibleSection
          title="Catalog coverage"
          description="See which saved filters narrow this channel's available clips."
        >
          <CoverageMeter coverage={coverage} />
        </CollapsibleSection>
      )}

      {/* Apply / Discard — only offered when the draft differs from what's saved. Apply is
          the seamless commit; Discard throws the draft away. */}
      {isDirty && (
        <div className="sticky bottom-0 flex items-center gap-2 rounded-lg border border-border bg-background/95 p-3 shadow-lg backdrop-blur">
          <Button variant="suggest" size="sm" disabled={isApplying} onClick={apply}>
            {isApplying ? "Applying…" : "Apply filler"}
          </Button>
          <Button variant="ghost" size="sm" disabled={isApplying} onClick={discard}>
            Discard
          </Button>
          <span className="ml-auto text-muted-foreground text-xs">Unsaved changes</span>
        </div>
      )}
    </div>
  );
};

export { ChannelFiller };
