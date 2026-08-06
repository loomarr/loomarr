import { settingsApi, unwrap } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui";
import { cn } from "@/lib";
import type { TunePanelProps } from "./tune-panel.type";

// TunePanel — the auto-file policy, inline on Incoming (the v2 mock's `toggleAuto` block).
//
// ⚠ Recreated from the mock rather than designed: the collapsed row (pulsing dot, work line,
// a `Tune` / `Hide settings` button), the "File automatically above ‹75|85|95› confidence" chip
// row, the checkbox toggles, and the monospace summary line are all its structure.
//
// ⚠ TWO PARTS OF THE MOCK ARE DELIBERATELY ABSENT, and both are decisions rather than oversights:
//
//   - The "Drop unidentifiable clips under ‹2|4|6›" chips. No setting reads them. `filler.
//     min_duration` exists but is a HARD REJECT at the scan boundary (V40's guard against a
//     2.9KB truncated download), not an auto-file policy — and its default of 10 sits outside
//     the mock's 2/4/6 range entirely. §15's rule is that a setting nothing reads does not
//     exist; the same applies to a control that writes nowhere. Recorded as open in PROGRESS.
//   - The mock's "−16 LUFS" in the loudness label. V42 ships that toggle against
//     `filler.target_lufs` (−23), because two targets in one system means a clip normalised on
//     file is corrected again at playout toward a different number.
//
// ⚠ Writes land IMMEDIATELY, not into a save buffer. This is the same inline-commit verb the
// wizard's playout step uses: the chips change what happens to the clips visible directly below
// them, so a staged edit the operator never confirms would leave the panel describing a policy
// that is not in force.
const CONFIDENCE_CHIPS = [75, 85, 95] as const;

const CONFIDENCE_KEY = "filler.autofile.min_confidence";
const LOUDNESS_KEY = "filler.autofile.normalize_loudness";

const TunePanel = ({ filed, needsYou }: TunePanelProps) => {
  const [open, setOpen] = useState(false);
  const queryClient = useQueryClient();

  const settings = settingsApi.useSettingsList({ query: { retry: false } });
  const entries = unwrap(settings.data, (b) => b.settings) ?? [];
  const entry = (key: string) => entries.find((e) => e.key === key);
  const boolOf = (key: string) => entry(key)?.value === "true";
  const confidence = Number(entry(CONFIDENCE_KEY)?.value ?? 85);

  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        await queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() });
      },
      onError: () => toast.error("Couldn't save that setting"),
    },
  });
  const write = (key: string, value: string) => patch.mutate({ data: { edits: { [key]: value } } });

  // ⚠ An env-pinned key is READ-ONLY here, exactly as it is in Settings (§3.1). Letting a chip
  // look clickable and then silently lose to the environment is worse than showing it disabled.
  const pinned = (key: string) => entry(key)?.provenance === "env";

  // ⚠ ONE toggle, not the mock's two. Its first — "Skip clips already in the catalog" — has no
  // setting behind it, and it needs none: content-addressed identity (V38c) means a clip already
  // in the catalog upserts onto its own row by hash. Skipping duplicates is not a policy an
  // operator can switch off, it is what the pipeline does. A checkbox implying otherwise would
  // offer a choice that does not exist.
  //
  // ⚠ The mock says "to −16 LUFS"; the shipped target is −23 and the label says so. Naming a
  // number the system does not use would be a lie on screen, which is worse than a delta.
  const toggles = [{ key: LOUDNESS_KEY, label: "Normalize loudness to −23 LUFS on file" }].filter(
    (t) => entry(t.key) !== undefined,
  );

  const summary = `Right now that files ${filed} ${filed === 1 ? "clip" : "clips"} without asking. ${
    needsYou > 0 ? `${needsYou} come to you.` : "Nothing needs you."
  }`;

  return (
    <div className="flex flex-col gap-3 rounded-xl border border-border bg-card px-[18px] py-[15px]">
      <div className="flex flex-wrap items-center gap-[11px]">
        {/* The mock's pulsing dot. `motion-safe` so a reduced-motion viewer gets a steady dot
            rather than the animation — and the visual suite kills animations outright, so this
            never destabilises a snapshot. */}
        <span className="size-2 shrink-0 rounded-full bg-signal motion-safe:animate-pulse" aria-hidden />
        <span className="font-semibold text-[13.5px]">Auto-filing</span>
        <Button
          variant="outline"
          size="sm"
          className="ml-auto"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
        >
          {open ? "Hide settings" : "Tune"}
        </Button>
      </div>
      <p className="text-[12.5px] text-muted-foreground leading-normal">
        Loomarr files what it recognises, drops junk, and skips anything already in the catalog.
      </p>

      {open && (
        <div className="flex flex-col gap-3 border-border border-t pt-[13px]">
          <div className="flex flex-wrap items-center gap-6">
            {/* ⚠ A real `<fieldset>`/`<legend>`, not a div with `role="group"` + aria-labelledby.
                The chips are ONE control with several options; unlabelled they read to a screen
                reader as three loose percentages with no hint of what they set. The first draft
                hand-wired the ARIA and biome's `useSemanticElements` rejected it — correctly:
                the native element announces the grouping without ARIA to keep in sync, which is
                the same lesson as the axe-passes-but-keyboard-broken case (adding ARIA can
                introduce the violation it was meant to prevent). `contents` keeps the mock's
                inline row, since fieldset carries default layout. */}
            <fieldset className="contents">
              <legend className="text-[12.5px] text-muted-foreground">File automatically above</legend>
              <div className="flex gap-[5px]">
                {CONFIDENCE_CHIPS.map((v) => (
                  <button
                    key={v}
                    type="button"
                    disabled={pinned(CONFIDENCE_KEY)}
                    aria-pressed={confidence === v}
                    onClick={() => write(CONFIDENCE_KEY, String(v))}
                    className={cn(
                      "rounded-full border px-[11px] py-[3px] font-mono text-[11.5px] transition-colors",
                      "disabled:cursor-not-allowed disabled:opacity-60",
                      confidence === v
                        ? "border-signal/45 bg-signal/15 text-signal"
                        : "border-border text-muted-foreground hover:text-foreground",
                    )}
                  >
                    {v}%
                  </button>
                ))}
              </div>
              <span className="text-[12.5px] text-muted-foreground">confidence</span>
            </fieldset>
          </div>

          <div className="flex flex-wrap gap-[22px]">
            {toggles.map((t) => {
              const on = boolOf(t.key);
              return (
                <label key={t.key} className="flex cursor-pointer items-center gap-2">
                  <input
                    type="checkbox"
                    checked={on}
                    disabled={pinned(t.key)}
                    onChange={() => write(t.key, on ? "false" : "true")}
                    className="size-[15px] cursor-pointer rounded-[4px] accent-signal disabled:cursor-not-allowed"
                  />
                  <span className="text-[12.5px]">{t.label}</span>
                </label>
              );
            })}
          </div>

          {/* ⚠ The destructive one gets a warning, and the mock has no equivalent because its
              toggle was a decoration. This rewrites the operator's files and cannot be undone;
              a checkbox that quiet would be the wrong affordance for that. */}
          {boolOf(LOUDNESS_KEY) && (
            <p className="text-[11.5px] text-onair-300">
              Clips are rewritten as they're filed. The original file is replaced and can't be recovered.
            </p>
          )}

          <p className="font-mono text-[11px] text-muted-foreground">{summary}</p>
        </div>
      )}
    </div>
  );
};

export { TunePanel };
