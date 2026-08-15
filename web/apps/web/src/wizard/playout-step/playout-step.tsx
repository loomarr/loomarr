import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as setupApi from "@loomarr/api/endpoints/setup";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import { useState } from "react";
import { SettingsFields } from "@/components/loomarr/settings/settings-fields";
import { ConnectionBlock } from "@/components/loomarr/setup/connection-block";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { PLAYOUT_INTERNAL, PLAYOUT_TUNARR, type PlayoutBackend } from "../steps";
import type { PlayoutStepProps } from "./playout-step.type";

// Wizard step 2 — "How should Loomarr play your channels?" (design §13, §9.1).
//
// ⚠ THIS STEP EXISTS TO STOP THE WIZARD PRESENTING A DECISION AS A DEPENDENCY. Before it,
// `tunarr` was a hardcoded blocking check with its own wiring step, so an operator on the
// DEFAULT path was asked to install and connect a second server they would never use, and
// could not continue until they did. The answer here decides which of the remaining steps
// exist at all, so it has to come before Connections.
//
// Deliberately two cards rather than a <select>: this is a fork with real consequences for
// what the operator installs, and a dropdown reads like a preference. It writes the ordinary
// `playout.backend` key through the same PATCH path Settings uses (config-design §6) — no
// parallel form system, and the choice stays re-editable in Settings forever.
const OPTIONS: {
  value: PlayoutBackend;
  title: string;
  tagline: string;
  points: string[];
}[] = [
  {
    value: PLAYOUT_INTERNAL,
    title: "Loomarr",
    tagline: "Recommended. Nothing else to install.",
    points: [
      "Your channels show up in your TV app like any other channel.",
      "Ads can play in the middle of a show, not just between shows.",
      "Uses some of this machine's processing power while someone is watching.",
    ],
  },
  {
    value: PLAYOUT_TUNARR,
    title: "Tunarr",
    tagline: "Pick this if you already run Tunarr.",
    points: [
      "Tunarr does the work of playing your channels.",
      "You'll point Tunarr at your library in a moment.",
      "Ads can only play between shows.",
    ],
  },
];

const PlayoutStep = ({ value, pinnedBy }: PlayoutStepProps) => {
  const queryClient = useQueryClient();

  // One of §9's inline-commit verbs: the answer reshapes the remaining steps, so it has to
  // land before the operator continues rather than staging into a save buffer they never see.
  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        await queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() });
      },
    },
  });

  const choose = (next: PlayoutBackend) => {
    if (next === value || pinnedBy) return;
    patch.mutate({ data: { edits: { "playout.backend": next } } });
  };

  // --- Tunarr's own connection form, shown inline once Tunarr is chosen ---
  const entries = useSettingsEntries();
  // Essentials only, exactly as the checklist does it (§6): advanced keys live in Settings.
  const tunarrEntries = entries.filter((e) => e.group === "connections.tunarr" && !e.advanced);
  const status = setupApi.useSetupStatus();
  const checks = unwrap(status.data, (b) => b.checks) ?? [];
  const standing = checks.find((c) => c.name === "tunarr");

  const [edits, setEdits] = useState<Record<string, string>>({});
  const [open, setOpen] = useState(true);
  const [testing, setTesting] = useState(false);
  const [tested, setTested] = useState<{ ok: boolean; hint?: string } | undefined>();
  const runTest = settingsApi.useSetupTest();

  const patchResults = unwrap(patch.data, (b) => b.results) ?? undefined;
  const dirty = Object.keys(edits).length > 0;
  // A just-run Test wins over the standing verdict, same precedence the checklist uses.
  const verdict = tested ?? (standing ? { ok: standing.ok, hint: standing.hint } : undefined);

  const save = () => patch.mutate({ data: { edits } });
  // Test evaluates PERSISTED settings, so unsaved edits must be written first or the operator
  // tests the OLD address right after typing a new one (the flavor-save bug, config-design §6).
  const test = async () => {
    setTesting(true);
    try {
      if (dirty) await patch.mutateAsync({ data: { edits } });
      const res = await runTest.mutateAsync({ data: { check: "tunarr" } });
      if (res.status === 200) setTested({ ok: res.data.ok, hint: res.data.hint });
    } catch {
      // patch/test surface their own errors; the prior verdict stays on screen.
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="flex flex-col gap-4">
      {/* ⚠ REAL RADIOS, inside a real <fieldset>. These are mutually exclusive answers to one
          question, so assistive tech should announce the question (the legend), the group,
          and which option is chosen — none of which a list of <button>s conveys. An earlier
          cut used role="radio" on buttons, which biome's useSemanticElements caught: it also
          had no radiogroup parent, so the options would have been announced with no question
          attached to them. Arrow-key navigation comes free with native inputs. */}
      <fieldset className="border-0 p-0" disabled={pinnedBy !== undefined}>
        <legend className="sr-only">How should Loomarr play your channels?</legend>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          {OPTIONS.map((option) => {
            const selected = option.value === value;
            return (
              // The whole card is the label, so the entire target is clickable rather than
              // just a small control the operator has to aim at.
              <label
                key={option.value}
                className={cn(
                  "flex cursor-pointer flex-col gap-2 rounded-lg border p-4 transition-colors",
                  selected ? "border-tune bg-tune/5" : "border-input hover:bg-accent",
                  pinnedBy && "cursor-not-allowed opacity-60",
                )}
              >
                <span className="flex items-center gap-2">
                  {/* ⚠ Visible, not `sr-only`. Hiding it made the control unreachable for
                      anything driving the real input (Playwright's .check() timed out on it),
                      and a hidden radio also gives a sighted keyboard user no focus ring to
                      follow. The card stays fully clickable because it is the <label>. */}
                  <input
                    type="radio"
                    name="playout-backend"
                    value={option.value}
                    checked={selected}
                    onChange={() => choose(option.value)}
                    className="size-4 shrink-0 accent-tune"
                  />
                  <span className="font-medium">{option.title}</span>
                </span>
                <span className="text-muted-foreground text-sm">{option.tagline}</span>
                <ul className="flex flex-col gap-1 text-muted-foreground text-sm">
                  {option.points.map((point) => (
                    <li key={point}>{point}</li>
                  ))}
                </ul>
              </label>
            );
          })}
        </div>
      </fieldset>

      {pinnedBy && (
        <p className="flex items-center gap-2 text-muted-foreground text-sm">
          <Badge className="gap-1">
            <Lock className="size-3" aria-hidden />
            set via environment
          </Badge>
          <span>
            {pinnedBy} decides this, so it can&rsquo;t be changed here. Unset it to choose in the app.
          </span>
        </p>
      )}

      {/* ⚠ Tunarr's own settings live HERE, under the choice that makes them relevant, rather
          than in Connections. Choosing Tunarr and then being sent to a different step to say
          where it is splits one decision across two screens. Keeping them together also means
          the Connections step never has to explain why a Tunarr block appeared.

          The SAME ConnectionBlock + SettingsFields the checklist uses, writing through the
          same PATCH path (config-design §6): no parallel form system, so env-pinning, the
          unlock affordance and validation all behave identically. */}
      {value === PLAYOUT_TUNARR && (
        <ConnectionBlock
          title="Tunarr"
          verdict={verdict}
          docHref={standing?.docHref}
          open={open}
          onToggle={() => setOpen((o) => !o)}
          action={
            <Button variant="outline" onClick={test} disabled={testing}>
              {testing ? "Testing…" : "Test connection"}
            </Button>
          }
        >
          <SettingsFields
            entries={tunarrEntries}
            values={edits}
            onChange={(key, v) => setEdits((p) => ({ ...p, [key]: v }))}
            results={patchResults}
          />
          {dirty && (
            <div className="mt-3">
              <Button onClick={save} disabled={patch.isPending}>
                {patch.isPending ? "Saving…" : "Save"}
              </Button>
            </div>
          )}
        </ConnectionBlock>
      )}

      <p className="text-muted-foreground text-sm">
        You can change this later in Settings, and set it per channel too.
      </p>
    </div>
  );
};

export { PlayoutStep };
