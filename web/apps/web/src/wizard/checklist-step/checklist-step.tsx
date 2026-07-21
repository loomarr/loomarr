import { settingsApi, setupApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { Check, ChevronDown, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { ErrorState, SettingsFields } from "@/components/loomarr";
import { Button } from "@/components/ui";
import { cn } from "@/lib";
import { useSettingsEntries } from "@/settings";
import { REQUIRED_CHECKS } from "../steps";
import type { ChecklistStepProps } from "./checklist-step.type";

// Wizard step 2 — "Connect your services" (config-design §6). The wizard IS the settings
// system: each block renders the relevant settings group's form (essentials only) with its
// live test inline — configure → validate → save, right here — NOT a read-only checklist
// that punts the operator off to Settings. It writes through the exact same PATCH path
// Settings uses. Only media server + Tunarr must pass to continue; the rest add features.
//
// Each connection is a collapsible section: its header shows a status dot + summary, and
// picking it (here or from the rail's sub-items) reveals its form with a slide-open. Blocks
// mirror Settings/Connections, minus advanced keys (the wizard shows the short path).
const BLOCKS = [
  { id: "media_server", group: "connections.media_server", title: "Media server" },
  { id: "tunarr", group: "connections.tunarr", title: "Tunarr" },
  { id: "requester", group: "connections.requester", title: "Requester (Seerr)" },
  { id: "tmdb", group: "connections.tmdb", title: "TMDB" },
] as const;

const ChecklistStep = ({ openId, onToggle }: ChecklistStepProps) => {
  const queryClient = useQueryClient();
  const entries = useSettingsEntries();
  const status = setupApi.useSetupStatus();

  const [edits, setEdits] = useState<Record<string, string>>({});
  const [testing, setTesting] = useState<string | undefined>();
  const [testResult, setTestResult] = useState<Record<string, { ok: boolean; hint?: string }>>({});
  const sectionRefs = useRef<Record<string, HTMLElement | null>>({});

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
  const test = async (check: string) => {
    setTesting(check);
    try {
      if (Object.keys(edits).length > 0) {
        await patch.mutateAsync({ data: { edits } });
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
      {BLOCKS.map((block) => {
        const blockEntries = byGroup(block.group);
        if (blockEntries.length === 0) return null;
        const open = block.id === openId;
        // Prefer a just-run Test result; fall back to the checklist's standing verdict.
        const live = testResult[block.id];
        const standing = checks.find((c) => c.name === block.id);
        const verdict = live ?? (standing ? { ok: standing.ok, hint: standing.hint } : undefined);
        const required = (REQUIRED_CHECKS as readonly string[]).includes(block.id);

        return (
          <section
            key={block.id}
            ref={(el) => {
              sectionRefs.current[block.id] = el;
            }}
            className="overflow-hidden rounded-lg border border-border"
          >
            <button
              type="button"
              onClick={() => onToggle?.(block.id)}
              aria-expanded={open}
              className="flex w-full cursor-pointer items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-static-800"
            >
              {/* Status dot: green when its check passes, muted otherwise. */}
              <span
                className={cn(
                  "flex size-5 shrink-0 items-center justify-center rounded-full border",
                  verdict?.ok ? "border-lock bg-lock-tint-15 text-lock" : "border-border text-static-400",
                )}
                aria-hidden
              >
                {verdict?.ok ? <Check className="size-3" /> : null}
              </span>
              <span className="font-medium text-sm">{block.title}</span>
              {!required && <span className="text-static-400 text-xs">optional</span>}
              <ChevronDown
                className={cn(
                  "ml-auto size-4 text-muted-foreground transition-transform",
                  open && "rotate-180",
                )}
                aria-hidden
              />
            </button>

            {/* The reveal: grid 0fr→1fr, so the form slides open with no fixed height. */}
            <div className="reveal" data-open={open}>
              <div className="reveal-inner">
                <div className="flex flex-col gap-4 border-border border-t px-4 py-4">
                  <SettingsFields
                    entries={blockEntries}
                    values={edits}
                    onChange={(key, value) => setEdits((p) => ({ ...p, [key]: value }))}
                    results={patchResults}
                  />
                  <div className="flex flex-wrap items-center gap-3">
                    <Button variant="outline" onClick={() => test(block.id)} disabled={testing !== undefined}>
                      {testing === block.id ? "Testing…" : "Test connection"}
                    </Button>
                    {verdict && (
                      <p
                        role="status"
                        className={cn(
                          "flex items-center gap-1.5 text-sm",
                          verdict.ok ? "text-lock" : "text-onair-300",
                        )}
                      >
                        {!verdict.ok && <X className="size-3.5" aria-hidden />}
                        {verdict.hint ?? (verdict.ok ? "Connection OK" : "Not connected yet")}
                      </p>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </section>
        );
      })}

      <p className="mt-1 text-muted-foreground text-xs">
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
