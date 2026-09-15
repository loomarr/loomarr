import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as setupApi from "@loomarr/api/endpoints/setup";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ErrorState } from "@/components/loomarr/feedback/error-state";
import { SettingsSaveBar } from "@/components/loomarr/settings/settings-save-bar";
import { useSettingsEdits } from "../settings-edits";

// One commit control for any settings workflow. Settings uses it across route tabs; Filler uses
// the same module on its operational-settings page, so moving a field does not invent a second
// save protocol.
const SettingsSaveBarHost = () => {
  const queryClient = useQueryClient();
  const { edits, clearEdits, resetEdits } = useSettingsEdits();
  const patch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async (response) => {
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() }),
          queryClient.invalidateQueries({ queryKey: setupApi.getSetupStatusQueryKey() }),
        ]);
        if (response.status !== 200) return;
        const saved = (response.data.results ?? [])
          .filter((result) => result.status === "saved")
          .map((result) => result.key);
        clearEdits(saved);
        const rejected = (response.data.results ?? []).filter((result) => result.status !== "saved");
        if (saved.length > 0) toast.success(saved.length === 1 ? "Change saved" : "Changes saved");
        if (rejected.length > 0) toast.error("Some changes need your attention");
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
