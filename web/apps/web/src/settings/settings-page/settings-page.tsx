import { settingsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { ErrorState, SettingsFields, SettingsSaveBar } from "@/components/loomarr";
import { Button } from "@/components/ui";
import type { SettingsPageProps } from "./settings-page.type";

// One Settings page: several blocks, ONE save bar (config-design §5). The page owns the
// edit state precisely because the save unit is the page — which is why SettingsFields is
// controlled rather than owning a form of its own.
//
// Only CHANGED keys are sent, for the same reason the wizard does it: a stored secret
// reads back empty (§4) and an empty-string PATCH would clear it (§9). Here that matters
// even more, since a page carries many keys the operator never touched.
const SettingsPage = ({ title, description, blocks, entries, children, footer }: SettingsPageProps) => {
  const queryClient = useQueryClient();
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [testing, setTesting] = useState<string | undefined>();
  const [testResult, setTestResult] = useState<Record<string, { ok: boolean; hint?: string }>>({});

  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        await queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() });
        setEdits({}); // saved values are the new baseline
      },
    },
  });
  const runTest = settingsApi.useSetupTest();

  const results = patch.data?.status === 200 ? (patch.data.data.results ?? []) : undefined;
  const byGroup = (group: string) => entries.filter((e) => e.group === group);

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

  return (
    <div className="flex h-full flex-col">
      <header className="border-border border-b px-6 py-4">
        <h1 className="font-semibold text-xl">{title}</h1>
        {description && <p className="mt-1 text-muted-foreground text-sm">{description}</p>}
      </header>

      <div className="flex flex-1 flex-col gap-8 overflow-auto p-6">
        {children}

        {blocks.map((block) => {
          const blockEntries = byGroup(block.group);
          if (blockEntries.length === 0) return null;
          const result = block.check ? testResult[block.check] : undefined;
          return (
            <section key={block.group} className="flex flex-col gap-4">
              <h2 className="font-semibold text-lg">{block.title}</h2>
              <SettingsFields
                entries={blockEntries}
                values={edits}
                onChange={(key, value) => setEdits((p) => ({ ...p, [key]: value }))}
                results={results}
              />
              {block.check && (
                <div className="flex items-center gap-3">
                  <Button
                    variant="outline"
                    onClick={() => test(block.check as string)}
                    disabled={testing !== undefined}
                  >
                    {testing === block.check ? "Testing…" : "Test connection"}
                  </Button>
                  {result && (
                    <p role="status" className={result.ok ? "text-lock text-sm" : "text-onair-300 text-sm"}>
                      {result.hint ?? (result.ok ? "Connection OK" : "Connection failed")}
                    </p>
                  )}
                </div>
              )}
            </section>
          );
        })}

        {footer}

        {patch.error != null && <ErrorState error={patch.error} />}
      </div>

      <SettingsSaveBar
        dirtyCount={Object.keys(edits).length}
        saving={patch.isPending}
        onDiscard={() => setEdits({})}
        onSave={() => patch.mutate({ data: { edits } })}
      />
    </div>
  );
};

export { SettingsPage };
