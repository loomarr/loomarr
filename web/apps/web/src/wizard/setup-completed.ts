import { settingsApi } from "@loomarr/api";
import type { QueryClient } from "@tanstack/react-query";

// The `setup.completed` registry key drives first-run routing: until it's set, `/` goes
// to the wizard (config-design §6). It's an ordinary setting, so it's read through the
// ordinary settings list and written through the ordinary PATCH.
const SETUP_COMPLETED_KEY = "setup.completed";

// Fails OPEN: any uncertainty (a member's 403, a 500, a missing key) reports "completed"
// so we never trap a non-admin — or a healthy install — inside an operator-only wizard.
// Only an explicit `false` from an admin-readable registry routes to setup.
const isSetupCompleted = async (queryClient: QueryClient): Promise<boolean> => {
  try {
    const res = await queryClient.ensureQueryData(
      settingsApi.getSettingsListQueryOptions({ query: { retry: false } }),
    );
    if (res.status !== 200) return true;
    const entry = res.data.settings?.find((s) => s.key === SETUP_COMPLETED_KEY);
    return entry?.value !== "false";
  } catch {
    return true;
  }
};

export { isSetupCompleted, SETUP_COMPLETED_KEY };
