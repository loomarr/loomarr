import * as channelsApi from "@loomarr/api/endpoints/channels";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { defaultGuideWindow } from "@/channels/guide-window";
import { isSetupCompleted } from "@/wizard/setup-completed";

// The app home (`/`). On a fresh instance the operator lands in the first-run wizard
// instead of an empty Channels page (config-design §6: until `setup.completed` is set,
// `/` routes to the wizard). isSetupCompleted fails open, so an ordinary member is never
// diverted into operator-only setup.
const Route = createFileRoute("/_authed/")({
  beforeLoad: async ({ context }) => {
    // Start the guide request NOW, alongside the setup check, rather than after the redirect
    // (#1396): the /guide loader would otherwise only begin once /v1/settings has answered. Same
    // quantised window as that loader, so it lands on the entry the page then reads. Not awaited,
    // and a wizard-bound redirect merely leaves one unused cache entry.
    void context.queryClient.prefetchQuery(
      channelsApi.getChannelGuideQueryOptions(defaultGuideWindow(Date.now())),
    );
    const completed = await isSetupCompleted(context.queryClient);
    throw redirect({ to: completed ? "/guide" : "/wizard" });
  },
});

export { Route };
