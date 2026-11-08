import * as settingsApi from "@loomarr/api/endpoints/settings";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { unwrap } from "@loomarr/api/unwrap";
import { invalidateSettings } from "@loomarr/core/events";
import { Action, Text } from "@loomarr/design-system";
import { LibraryWizardStep } from "@loomarr/ui";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { SettingsFields } from "@/components/loomarr/settings/settings-fields";
import { useLibraryFolders } from "@/library/use-library-folders";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { ChecklistStep } from "../checklist-step";
import { useArrsFromRequester } from "../use-arrs-from-requester";
import type { LibraryStepProps } from "./library-step.type";

// Wizard step: "Where's your media?" (#2151 decision 30). The folders Loomarr found through the
// media server, Sonarr and Radarr, each with Use this folder. Choosing one starts adding the media;
// the step shows the progress and never waits for it. Continue needs one folder Loomarr can open.
//
// Sonarr and Radarr are connected here, beside the folders they name: filled in from Seerr when
// Seerr knows them, and editable when the address Seerr uses isn't one Loomarr can reach. Their
// webhooks need Loomarr's own address, asked here when nothing has set it yet.

const LibraryStep = ({ backend }: LibraryStepProps) => {
  const folders = useLibraryFolders();
  const entries = useSettingsEntries();
  const value = (key: SettingKey) => entries.find((e) => e.key === key)?.value ?? "";
  const arrsMissing =
    entries.length > 0 && (value(SettingKey.SonarrURL) === "" || value(SettingKey.RadarrURL) === "");
  const fill = useArrsFromRequester({ missing: arrsMissing, onFilled: () => void folders.onCheckAgain() });
  const [openArr, setOpenArr] = useState<string | undefined>();
  const body = folders.roots;
  const current = folders.status;

  if (folders.error && !body) {
    return <ErrorState error={folders.error} onRetry={() => void folders.refresh()} />;
  }
  if (!body || !current) return null;

  const anArr = value(SettingKey.SonarrURL) !== "" || value(SettingKey.RadarrURL) !== "";
  return (
    <>
      {folders.error ? <ErrorState error={folders.error} /> : null}
      <LibraryWizardStep
        checkedJustNow={folders.checked}
        checking={folders.checking}
        folders={body.roots}
        onAddFolder={folders.onAddFolder}
        onCheckAgain={folders.onCheckAgain}
        onUseFolder={folders.onUseFolder}
        pathMap={body.pathMap}
        scan={current.scan}
      />
      <Text textRole="label" tone="secondary">
        Sonarr and Radarr
      </Text>
      <Text textRole="body" tone="secondary">
        Recommended: with Sonarr and Radarr, Loomarr knows your files faster and for sure, and new downloads
        show up right away.
      </Text>
      {fill.filled.length > 0 ? (
        <Text textRole="body" tone="secondary">
          Filled in from Seerr:{" "}
          {fill.filled.map((a) => (a.app === "sonarr" ? "Sonarr" : "Radarr")).join(" and ")}. If Loomarr
          can&rsquo;t reach one at that address, change it here.
        </Text>
      ) : null}
      {fill.error ? <ErrorState error={fill.error} /> : null}
      <ChecklistStep
        backend={backend}
        only={["sonarr", "radarr"]}
        openId={openArr}
        onToggle={(id) => setOpenArr((open) => (open === id ? undefined : id))}
      />
      {anArr && value(SettingKey.ServerPublicURL) === "" ? <LoomarrAddress /> : null}
    </>
  );
};

// LoomarrAddress asks for the address Sonarr and Radarr send new downloads to, the same setting the
// Playout step asks for when Loomarr plays the channels.
const LoomarrAddress = () => {
  const queryClient = useQueryClient();
  const entries = useSettingsEntries().filter((e) => e.key === SettingKey.ServerPublicURL);
  const [edits, setEdits] = useState<Record<string, string>>({});
  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        await invalidateSettings(queryClient);
        setEdits({});
      },
    },
  });
  return (
    <>
      <SettingsFields
        entries={entries}
        values={edits}
        onChange={(key, v) => setEdits((previous) => ({ ...previous, [key]: v }))}
        results={unwrap(patch.data, (b) => b.results) ?? undefined}
        descriptionIds={{ [SettingKey.ServerPublicURL]: "library-step-address" }}
      />
      <Text textRole="body" tone="secondary" nativeID="library-step-address">
        Sonarr and Radarr tell Loomarr about new downloads at this address, so new episodes show up right
        away.
      </Text>
      {Object.keys(edits).length > 0 ? (
        <Action onPress={() => patch.mutate({ data: { edits } })} disabled={patch.isPending} tone="primary">
          {patch.isPending ? "Saving…" : "Save address"}
        </Action>
      ) : null}
      {patch.error != null && <ErrorState error={patch.error} />}
    </>
  );
};

export { LibraryStep };
