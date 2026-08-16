import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as setupApi from "@loomarr/api/endpoints/setup";
import * as systemApi from "@loomarr/api/endpoints/system";
import { unwrap } from "@loomarr/api/unwrap";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { SettingsFields } from "@/components/loomarr/settings/settings-fields";
import { ConnectionBlock } from "@/components/loomarr/setup/connection-block";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { Button } from "@/components/ui/button";
import { useSettingsEdits } from "../settings-edits";
import type { SettingsPageProps } from "./settings-page.type";

// One Settings page: several blocks, ONE save bar (config-design §5). The page owns the
// edit state precisely because the save unit is the page — which is why SettingsFields is
// controlled rather than owning a form of its own.
//
// A block that declares a `check` is a CONNECTION: it renders as a self-diagnosing
// ConnectionBlock (the wizard's shell) — a status dot + inline Test verdict + Fix link,
// collapsed by default with the first failure open, so the page starts with one thing to fix.
// A block with no check (the AI/Channels/… field groups) stays a flat titled
// section: there's no health state to triage, so collapsing it would only hide fields.
//
// Only CHANGED keys are sent, for the same reason the wizard does it: a stored secret
// reads back empty (§4) and an empty-string PATCH would clear it (§9). Here that matters
// even more, since a page carries many keys the operator never touched.
const SettingsPage = ({ title, description, blocks, entries, children, footer }: SettingsPageProps) => {
  const queryClient = useQueryClient();
  // ⚠ The edit buffer lives in the LAYOUT, not here (V9). Held locally, it died on every tab
  // switch and took the operator's unsaved edits with it — silently, which is the worst way to
  // lose work. The save unit is still "everything staged", the bar is still one bar; what
  // changed is that the buffer outlives this component.
  const { edits, setEdit, resetEdits } = useSettingsEdits();
  const [testing, setTesting] = useState<string | undefined>();
  const [testResult, setTestResult] = useState<Record<string, { ok: boolean; hint?: string }>>({});
  // Which connection blocks are expanded. Seeded once from the checklist (first failure open,
  // everything else collapsed); after that the operator drives it by clicking headers.
  const [openBlocks, setOpenBlocks] = useState<Record<string, boolean> | undefined>();

  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        // A saved connection key can flip its check — refresh both, like the wizard.
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() }),
          queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() }),
          // Saving provider/base/key must refresh the hosted catalog. Otherwise the
          // picker keeps its pre-save "add a key" snapshot until a full reload.
          queryClient.invalidateQueries({ queryKey: systemApi.getSystemLlmStatusQueryKey() }),
        ]);
        resetEdits(); // saved values are the new baseline
      },
    },
  });
  // Taking a key back from the environment (config-design §3.1). One of §9's inline-commit
  // verbs: it commits immediately rather than staging into the save buffer, because it
  // changes WHO MANAGES the key rather than its value — and a field that stayed locked until
  // Save could not be edited to produce something to save.
  const envOverride = settingsApi.useSettingsEnvOverride({
    mutation: {
      onSuccess: async () => {
        // Re-read: the claim changes provenance and lock state for that key, and unlocking
        // seeds a stored value the field must now render.
        await queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() });
      },
    },
  });
  const onEnvOverride = (key: string, enabled: boolean) => {
    envOverride.mutate({ key, data: { enabled } });
  };
  const runTest = settingsApi.useSetupTest();
  // Standing verdicts for connection blocks — only fetched when a block needs one (a page
  // with no checks doesn't probe). The page reports each block's status without a click.
  const hasChecks = blocks.some((b) => b.check);
  const status = setupApi.useSetupStatus({ query: { enabled: hasChecks } });
  const checks = unwrap(status.data, (b) => b.checks) ?? [];
  const standingFor = (check?: string) => (check ? checks.find((c) => c.name === check) : undefined);

  const results = patch.data?.status === 200 ? (patch.data.data.results ?? []) : undefined;
  const byGroup = (group: string) => entries.filter((e) => e.group === group);
  const entriesFor = (block: SettingsPageProps["blocks"][number]) => {
    const grouped = byGroup(block.group);
    if (!block.keys) return grouped;
    const byKey = new Map(grouped.map((entry) => [entry.key, entry]));
    return block.keys.flatMap((key) => {
      const entry = byKey.get(key);
      return entry ? [entry] : [];
    });
  };

  // The live value of a key, honoring an unsaved edit over the resolved value — the same
  // rule SettingsFields uses. Shared with the footer render prop so a consequence panel
  // (the AI model picker) reacts to an unsaved provider switch immediately.
  const liveValue = (key: string): string => edits[key] ?? entries.find((e) => e.key === key)?.value ?? "";

  // Seed the open-set once the checklist first arrives: open only the first failing/unknown
  // block. Every other failure stays explicit in its collapsed header; expanding them all made
  // a fresh install a wall of fields rather than a triage flow. If nothing is broken, open the
  // first block so a fully-healthy page still shows one editable connection.
  // Guarded by `openBlocks === undefined` so it runs exactly once — after that the operator
  // owns which blocks are open, and a later refetch never yanks a block shut under them.
  const checksReady = hasChecks && checks.length > 0;
  // biome-ignore lint/correctness/useExhaustiveDependencies: seed exactly once when checks first arrive; blocks/standingFor are read at that moment, never as reactive deps
  useEffect(() => {
    if (!checksReady || openBlocks !== undefined) return;
    const connectionBlocks = blocks.filter((b) => b.check);
    const broken = connectionBlocks.filter((b) => !standingFor(b.check)?.ok);
    const initial: Record<string, boolean> = {};
    for (const b of connectionBlocks) initial[b.group] = false;
    if (broken[0]) initial[broken[0].group] = true;
    else if (connectionBlocks[0]) initial[connectionBlocks[0].group] = true;
    setOpenBlocks(initial);
  }, [checksReady]);

  // Connection forms are an accordion: moving to another service closes the previous one. This
  // keeps the page focused even after the initial triage seed instead of letting the wall of fields
  // accumulate again through ordinary exploration. Clicking the open header still closes it.
  const toggleBlock = (group: string) =>
    setOpenBlocks((previous) => {
      const next = Object.fromEntries(
        blocks.filter((block) => block.check).map((block) => [block.group, false]),
      );
      next[group] = !(previous?.[group] ?? false);
      return next;
    });

  // Test checks PERSISTED settings (/v1/setup/test takes only a check name and evaluates
  // what's saved) — so unsaved edits would be tested against the OLD stored values. Typing a
  // token and pressing Test would then probe with the empty stored token and 401, even though
  // the right value is on screen. Save first when dirty, so Test always checks what you see.
  // (This mirrors the wizard's checklist-step, which fixed the identical trap.) A save that
  // fails surfaces via patch.error and leaves the edits for a retry.
  const test = async (check: string) => {
    setTesting(check);
    try {
      if (Object.keys(edits).length > 0) {
        await patch.mutateAsync({ data: { edits } });
      }
      const res = await runTest.mutateAsync({ data: { check } });
      if (res.status === 200) {
        setTestResult((p) => ({ ...p, [check]: { ok: res.data.ok, hint: res.data.hint } }));
      }
    } catch {
      // patch/test surface their own error UI (patch.error below); the prior verdict stays.
    } finally {
      setTesting(undefined);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={title} description={description} />

      <div className="flex flex-1 flex-col gap-8 overflow-auto p-6">
        {children}

        {/* Preserve the declared information hierarchy. A prior two-pass renderer moved every
            checked block before every ordinary one, which put the 45-field Filler operation
            panel above Scheduling even though Defaults declared Scheduling first. */}
        {blocks.map((block) => {
          const blockEntries = entriesFor(block);
          if (blockEntries.length === 0) return null;
          if (block.check) {
            // A just-run Test wins; otherwise fall back to the checklist's standing
            // verdict, so a block reports its health without a click (like the wizard).
            const live = testResult[block.check];
            const standing = standingFor(block.check);
            const verdict = live ?? (standing ? { ok: standing.ok, hint: standing.hint } : undefined);
            return (
              <div key={block.title} className="flex flex-col gap-3">
                <ConnectionBlock
                  title={block.title}
                  optional={block.optional}
                  {...(block.footer ? { footer: block.footer } : {})}
                  verdict={verdict}
                  docHref={standing?.docHref}
                  open={openBlocks?.[block.group] ?? false}
                  onToggle={() => toggleBlock(block.group)}
                  action={
                    <Button
                      variant="outline"
                      onClick={() => test(block.check as string)}
                      disabled={testing !== undefined}
                    >
                      {testing === block.check ? "Testing…" : "Test connection"}
                    </Button>
                  }
                >
                  {block.description && (
                    <p className="text-muted-foreground text-sm leading-relaxed">{block.description}</p>
                  )}
                  <SettingsFields
                    entries={blockEntries}
                    values={edits}
                    onChange={setEdit}
                    results={results}
                    onEnvOverride={onEnvOverride}
                    disabledReasons={block.disabledReasons}
                  />
                </ConnectionBlock>
              </div>
            );
          }
          return (
            <section key={block.title} className="flex flex-col gap-4">
              <div>
                <h2 className="font-semibold text-lg">{block.title}</h2>
                {block.description && <p className="text-muted-foreground text-sm">{block.description}</p>}
              </div>
              <SettingsFields
                entries={blockEntries}
                values={edits}
                onChange={setEdit}
                results={results}
                onEnvOverride={onEnvOverride}
                disabledReasons={block.disabledReasons}
              />
            </section>
          );
        })}

        {typeof footer === "function" ? footer({ liveValue, setEdit }) : footer}

        {patch.error != null && <ErrorState error={patch.error} />}
      </div>

      {/* The save bar lives in the LAYOUT now (V10), not here: All settings is a tab that is
          not a SettingsPage, and an edit staged there had no Save button on screen. One bar
          above the outlet also means two pages can never render two bars. */}
    </div>
  );
};

export { SettingsPage };
