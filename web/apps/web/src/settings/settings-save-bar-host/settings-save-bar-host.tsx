import { settingsApi, setupApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { ErrorState, SettingsSaveBar } from "@/components/loomarr";
import { useSettingsEdits } from "../settings-edits";

// One commit control for any settings workflow. Settings uses it across route tabs; Filler uses
// the same module on its operational-settings page, so moving a field does not invent a second
// save protocol.
const SettingsSaveBarHost = () => {
  const queryClient = useQueryClient();
  const { edits, resetEdits } = useSettingsEdits();
  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() }),
          queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() }),
        ]);
        resetEdits();
      },
    },
  });

  return (
    <>
      {patch.error != null && (
        <div className="px-6 pb-2">
          <ErrorState error={patch.error} />
        </div>
      )}
      <SettingsSaveBar
        dirtyCount={Object.keys(edits).length}
        saving={patch.isPending}
        onDiscard={resetEdits}
        onSave={() => patch.mutate({ data: { edits } })}
      />
    </>
  );
};

export { SettingsSaveBarHost };
