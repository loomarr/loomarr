import * as channelsApi from "@loomarr/api/endpoints/channels";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { defaultGuideWindow } from "@/channels/guide-window";
import { isSetupCompleted } from "@/wizard/setup-completed";

// The app home (`/`) lands on Home, for both roles (#1659). On a fresh instance the operator lands
// in the first-run wizard instead (config-design §6: until `setup.completed` is set, `/` routes to
// the wizard). isSetupCompleted fails open, so an ordinary member is never diverted into
// operator-only setup.
const Route = createFileRoute("/_authed/")({
  beforeLoad: async ({ context }) => {
    // Start the guide request NOW, alongside the setup check, rather than after the redirect
    // (#1396): Home reads the same quantised window as the Guide, so it lands on the entry both
    // pages read. Not awaited, and a wizard-bound redirect merely leaves one unused cache entry.
    void context.queryClient.prefetchQuery(
      channelsApi.getChannelGuideQueryOptions(defaultGuideWindow(Date.now())),
    );
    const completed = await isSetupCompleted(context.queryClient);
    throw redirect({ to: completed ? "/dashboard" : "/wizard" });
  },
});

export { Route };
