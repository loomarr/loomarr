import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as setupApi from "@loomarr/api/endpoints/setup";
import type { SettingEntry } from "@loomarr/api/models/settingEntry";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { unwrap } from "@loomarr/api/unwrap";
import { invalidateSettings } from "@loomarr/core/events";
import { Action } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { SettingsFields } from "@/components/loomarr/settings/settings-fields";
import { ConnectionBlock } from "@/components/loomarr/setup/connection-block";
import { blockTitle } from "@/settings/provider-title";
import { everyEditSaved } from "@/settings/saved-edits";
import { useSettingsDraft } from "@/settings/settings-draft";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { PLAYOUT_INTERNAL, requiredChecks } from "../steps";
import { WizardAiBlock } from "../wizard-ai-block";
import type { ChecklistStepProps, ConnectionBlockId } from "./checklist-step.type";

// Wizard step 2 — "Connect your services" (config-design §6). The wizard IS the settings
// system: each block renders the relevant settings group's form (essentials only) with its
// live test inline — configure → validate → save, right here — NOT a read-only checklist
// that punts the operator off to Settings. It writes through the exact same PATCH path
// Settings uses. Which blocks must pass depends on the playout backend (§9.1): the media
// server alone when Loomarr streams, plus Tunarr when Tunarr does. The rest add features.
//
// Each connection is a collapsible section: its header shows a status dot + summary, and
// picking it (here or from the rail's sub-items) reveals its form with a slide-open. Blocks
// mirror Settings/Connections, minus advanced keys (the wizard shows the short path).
// `custom` blocks render a bespoke body (their own probe/action) instead of a settings-group
// form + Test button — used by AI, which auto-detects Ollama and drives its own picker.
const BLOCKS = [
  { id: "media_server", group: "connections.media_server", title: "Media server" },
  { id: "tunarr", group: "connections.tunarr", title: "Tunarr" },
  // Requester + AI titles gain the SAVED provider suffix at render (see titleFor); the base
  // here carries no suffix until one is chosen + saved (§6).
  { id: "requester", group: "connections.requester", title: "Requester" },
  { id: "sonarr", group: "connections.sonarr", title: "Sonarr" },
  { id: "radarr", group: "connections.radarr", title: "Radarr" },
  { id: "tmdb", group: "connections.tmdb", title: "TMDB" },
  { id: "ai", title: "AI", custom: true },
] as const;

// ⚠ Tunarr NEVER appears here. On the internal path it is not part of the install at all; on
// the Tunarr path its form lives on the Playout step, directly under the choice that makes it
// relevant (design §13). Splitting "which one plays your channels" from "where is it" across
// two screens made one decision feel like two, and left Connections showing a block whose
// reason for existing was three steps back.
//
// It stays in BLOCKS because `requiredChecks` and the footer copy still name it: on the Tunarr
// path it is genuinely a blocking check, just not one configured from this screen.
//
// Sonarr and Radarr are the library's managers (#2151 decision 29), so they live on "Where's your
// media?", beside the folders they name, as they live under Settings → Library.
const LIBRARY_MANAGERS: readonly ConnectionBlockId[] = ["sonarr", "radarr"];
const blocksFor = (only?: readonly ConnectionBlockId[]): (typeof BLOCKS)[number][] =>
  BLOCKS.filter((b) =>
    only
      ? (only as readonly string[]).includes(b.id)
      : b.id !== "tunarr" && !(LIBRARY_MANAGERS as readonly string[]).includes(b.id),
  );

// titleFor appends the chosen-provider suffix for the provider-selected blocks (requester,
// AI), reading the saved value from the settings entries; every other block keeps its title.
const titleFor = (block: (typeof BLOCKS)[number], entries: SettingEntry[]): string => {
  if (block.id === "requester") return blockTitle(entries, block.title, SettingKey.RequesterProvider);
  if (block.id === "ai") return blockTitle(entries, block.title, SettingKey.LLMProvider);
  return block.title;
};

const ChecklistStep = ({ openId, onToggle, backend = PLAYOUT_INTERNAL, only }: ChecklistStepProps) => {
  const queryClient = useQueryClient();
  const entries = useSettingsEntries();
  const status = setupApi.useSetupStatus();

  const { edits, setEdit, dropSaved, resetEdits } = useSettingsDraft();
  const [testing, setTesting] = useState<string | undefined>();
  const [testResult, setTestResult] = useState<Record<string, { ok: boolean; hint?: string }>>({});
  const sectionRefs = useRef<Record<string, HTMLElement | null>>({});

  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async (response, { data }) => {
        // Field values AND the server checklist (a saved key can flip a check) refetch together.
        await invalidateSettings(queryClient);
        // Saved values are the new baseline. A 200 answers per key, so a rejected or pinned value
        // stays in the form, editable, beside the message the results give for it.
        dropSaved(response, data.edits);
      },
    },
  });
  // ⚠ THE CASE THIS WHOLE FEATURE EXISTS FOR (config-design §3.1). An operator puts a value
  // in `.env` to get the app booting, reaches this step, and finds the field they need to
  // correct is read-only — with no route forward but editing a file on the host and
  // restarting. On an appliance-style install that is a dead end, so the wizard offers the
  // same take-it-back affordance Settings does.
  const envOverride = settingsApi.useSettingsEnvOverride({
    mutation: {
      onSuccess: async () => {
        // Taking a key back can change the resolved value, and a resolved value can flip a check.
        await invalidateSettings(queryClient);
      },
    },
  });
  const runTest = settingsApi.useSetupTest();

  const checks = unwrap(status.data, (b) => b.checks) ?? [];
  // Essentials only in the wizard (§6): advanced keys live in Settings.
  const byGroup = (group: string) => entries.filter((e) => e.group === group && !e.advanced);
  const patchResults = unwrap(patch.data, (b) => b.results) ?? undefined;

  // When the open block changes (e.g. clicked in the rail), bring it into view. Guarded
  // because scrollIntoView isn't universally available (jsdom, older engines) — a missing
  // scroll must never break the reveal.
  useEffect(() => {
    if (!openId) return;
    sectionRefs.current[openId]?.scrollIntoView?.({ behavior: "smooth", block: "nearest" });
  }, [openId]);

  const save = () => patch.mutate({ data: { edits } });
  // Test checks PERSISTED settings (config-design §6: /v1/setup/test takes only a check
  // name and evaluates what's saved) — so unsaved edits would be tested against the old
  // (here: empty) values, which reads as "set a media server flavor" right after you
  // picked one. Save first when dirty, so Test always checks what's on screen. `edits`
  // captured before the save clears it; onError leaves the edits for the operator to retry.
  // If the server rejected any value, the test is skipped: it would only judge the old settings.
  const test = async (check: string) => {
    setTesting(check);
    try {
      if (Object.keys(edits).length > 0) {
        const saved = await patch.mutateAsync({ data: { edits } });
        if (!everyEditSaved(saved, edits)) return;
      }
      const res = await runTest.mutateAsync({ data: { check } });
      if (res.status === 200)
        setTestResult((p) => ({ ...p, [check]: { ok: res.data.ok, hint: res.data.hint } }));
    } catch {
      // patch/test surface their own error UI (patch.error below; test verdict stays prior).
    } finally {
      setTesting(undefined);
    }
  };

  const dirty = Object.keys(edits).length > 0;

  return (
    <div className="flex flex-col gap-3">
      {blocksFor(only).map((block) => {
        const custom = "custom" in block && block.custom;
        // "group" in block narrows the union so TS knows block.group exists (the AI block is
        // custom and carries no group); a custom block has no settings-group entries.
        const blockEntries = "group" in block ? byGroup(block.group) : [];
        // A settings-group block with no essentials to show is hidden; a custom block
        // (AI) has no group entries by design and always renders.
        if (!custom && blockEntries.length === 0) return null;
        const open = block.id === openId;
        // The AI block's server-side check is named "llm" (§8.1); every other block's check
        // name matches its id. Map so the AI block shows its real standing verdict.
        const checkName = block.id === "ai" ? "llm" : block.id;
        // Prefer a just-run Test result; fall back to the checklist's standing verdict.
        const live = testResult[block.id];
        const standing = checks.find((c) => c.name === checkName);
        const verdict =
          live ??
          (standing ? { ok: standing.ok, hint: standing.hint, notSetUp: standing.notSetUp } : undefined);
        const required = requiredChecks(backend).includes(block.id);

        return (
          // Wrapped so the open block can still be scrolled into view (ConnectionBlock owns
          // its own <section>; the wizard's rail-driven scroll targets this ref'd div).
          <div
            key={block.id}
            ref={(el) => {
              sectionRefs.current[block.id] = el;
            }}
          >
            <ConnectionBlock
              title={titleFor(block, entries)}
              optional={!required}
              verdict={verdict}
              testing={testing === block.id}
              docHref={standing?.docHref}
              open={open}
              onToggle={() => onToggle?.(block.id)}
              // A custom block owns its own actions (AI auto-detects + drives its picker),
              // so it gets no shared Test button.
              action={
                custom ? undefined : (
                  <Action disabled={testing !== undefined} onPress={() => test(block.id)} variant="outline">
                    {testing === block.id ? "Testing…" : "Test connection"}
                  </Action>
                )
              }
            >
              {custom ? (
                <WizardAiBlock />
              ) : (
                <SettingsFields
                  entries={blockEntries}
                  values={edits}
                  onChange={setEdit}
                  results={patchResults}
                  onEnvOverride={(key, enabled) => envOverride.mutate({ key, data: { enabled } })}
                />
              )}
            </ConnectionBlock>
          </div>
        );
      })}

      {/* Names the blocks that actually gate THIS install. Reading the derived set rather
          than a hardcoded pair is what keeps the sentence honest on the internal path, where
          it would otherwise promise the operator a Tunarr requirement that no longer exists. */}
      {only === undefined && (
        <p className="text-muted-foreground mt-1 text-xs">
          {requiredChecks(backend)
            .map((c) => BLOCKS.find((b) => b.id === c)?.title ?? c)
            .join(" and ")}{" "}
          must pass to continue. The rest add features you can wire up later, and Settings re-runs these
          checks for the life of the install.
        </p>
      )}

      {patch.error != null && <ErrorState error={patch.error} />}

      {/* One save for the step's edits (§6: the wizard saves per step). Only changed keys
          are sent — a stored secret reads back empty and an empty PATCH would clear it. */}
      {dirty && (
        <div className="flex items-center gap-3">
          <Action disabled={patch.isPending} onPress={save}>
            {patch.isPending ? "Saving…" : "Save changes"}
          </Action>
          <Action disabled={patch.isPending} onPress={resetEdits} variant="ghost">
            Discard
          </Action>
        </div>
      )}
    </div>
  );
};

export { ChecklistStep };
