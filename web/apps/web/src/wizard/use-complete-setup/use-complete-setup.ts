import { settingsApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { SETUP_COMPLETED_KEY } from "../setup-completed";

interface FinishOptions {
  to: "/suggest" | "/channels";
  // Prefills the Suggest workspace from a template (§13's blank-page killer).
  intent?: string;
}

// Closing the wizard is one act: flip `setup.completed` so `/` stops routing here
// (config-design §6), then leave. It writes through the ordinary PATCH path — the flag
// is a setting like any other — and the settings cache is invalidated so the `/` guard
// re-reads truth rather than a stale "not completed".
const useCompleteSetup = () => {
  const router = useRouter();
  const queryClient = useQueryClient();
  const patch = settingsApi.useSettingsPatch();

  const finish = ({ to, intent }: FinishOptions) => {
    patch.mutate(
      { data: { edits: { [SETUP_COMPLETED_KEY]: "true" } } },
      {
        onSuccess: async () => {
          await queryClient.invalidateQueries({ queryKey: settingsApi.getSettingsListQueryKey() });
          const search = intent ? `?intent=${encodeURIComponent(intent)}` : "";
          router.history.push(`${to}${search}`);
        },
      },
    );
  };

  return { finish, isPending: patch.isPending, error: patch.error };
};

export { useCompleteSetup };
