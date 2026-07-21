import { settingsApi, setupApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { ErrorState, SettingsFields } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { useSettingsEntries } from "@/settings";
import { REQUIRED_CHECKS } from "../steps";

// Wizard step 2 — "Connect your services" (config-design §6). The wizard IS the settings
// system: each block renders the relevant settings group's form (essentials only) with its
// live test inline — configure → validate → save, right here — NOT a read-only checklist
// that punts the operator off to Settings. It writes through the exact same PATCH path
// Settings uses. Only media server + Tunarr must pass to continue; the rest add features.
//
// Blocks mirror Settings/Connections, minus advanced keys (the wizard shows the short
// path). `check` is the server probe Test runs — the honest "did it actually work".
const BLOCKS = [
  { group: "connections.media_server", title: "Media server", check: "media_server" },
  { group: "connections.tunarr", title: "Tunarr", check: "tunarr" },
  { group: "connections.requester", title: "Requester", check: "requester" },
  { group: "connections.tmdb", title: "TMDB", check: "tmdb" },
] as const;

const ChecklistStep = () => {
  const queryClient = useQueryClient();
  const entries = useSettingsEntries();
  const status = setupApi.useSetupStatus();

  const [edits, setEdits] = useState<Record<string, string>>({});
  const [testing, setTesting] = useState<string | undefined>();
  const [testResult, setTestResult] = useState<Record<string, { ok: boolean; hint?: string }>>({});

  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        // Refresh field values AND the server checklist — a saved key can flip a check.
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() }),
          queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() }),
        ]);
        setEdits({}); // saved values are the new baseline
      },
    },
  });
  const runTest = settingsApi.useSetupTest();

  const checks = status.data?.status === 200 ? (status.data.data.checks ?? []) : [];
  // Essentials only in the wizard (§6): advanced keys live in Settings.
  const byGroup = (group: string) => entries.filter((e) => e.group === group && !e.advanced);
  const patchResults = patch.data?.status === 200 ? (patch.data.data.results ?? undefined) : undefined;

  const save = () => patch.mutate({ data: { edits } });
  const test = (check: string) => {
    setTesting(check);
    runTest.mutate(
      { data: { check } },
      {
        onSuccess: (res) => {
          if (res.status === 200)
            setTestResult((p) => ({ ...p, [check]: { ok: res.data.ok, hint: res.data.hint } }));
          setTesting(undefined);
        },
        onError: () => setTesting(undefined),
      },
    );
  };

  const dirty = Object.keys(edits).length > 0;

  return (
    <div className="flex flex-col gap-8">
      {BLOCKS.map((block) => {
        const blockEntries = byGroup(block.group);
        if (blockEntries.length === 0) return null;
        // Prefer a just-run Test result; fall back to the checklist's standing verdict.
        const live = testResult[block.check];
        const standing = checks.find((c) => c.name === block.check);
        const verdict = live ?? (standing ? { ok: standing.ok, hint: standing.hint } : undefined);
        return (
          <section key={block.group} className="flex flex-col gap-4">
            <h2 className="font-semibold text-lg">{block.title}</h2>
            <SettingsFields
              entries={blockEntries}
              values={edits}
              onChange={(key, value) => setEdits((p) => ({ ...p, [key]: value }))}
              results={patchResults}
            />
            <div className="flex flex-wrap items-center gap-3">
              <Button variant="outline" onClick={() => test(block.check)} disabled={testing !== undefined}>
                {testing === block.check ? "Testing…" : "Test connection"}
              </Button>
              {verdict && (
                <p role="status" className={verdict.ok ? "text-lock text-sm" : "text-onair-300 text-sm"}>
                  {verdict.hint ?? (verdict.ok ? "Connection OK" : "Not connected yet")}
                </p>
              )}
            </div>
          </section>
        );
      })}

      <p className="text-muted-foreground text-xs">
        {REQUIRED_CHECKS.map((c) => (c === "media_server" ? "Media server" : "Tunarr")).join(" and ")} must
        pass to continue. The rest add features you can wire up later — Settings re-runs these checks for the
        life of the install.
      </p>

      {patch.error != null && <ErrorState error={patch.error} />}

      {/* One save for the step's edits (§6: the wizard saves per step). Only changed keys
          are sent — a stored secret reads back empty and an empty PATCH would clear it. */}
      {dirty && (
        <div className="flex items-center gap-3">
          <Button onClick={save} disabled={patch.isPending}>
            {patch.isPending ? "Saving…" : "Save changes"}
          </Button>
          <Button variant="ghost" onClick={() => setEdits({})} disabled={patch.isPending}>
            Discard
          </Button>
        </div>
      )}
    </div>
  );
};

export { ChecklistStep };
