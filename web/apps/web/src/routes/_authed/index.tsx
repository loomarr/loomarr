import { createFileRoute, redirect } from "@tanstack/react-router";
import { isSetupCompleted } from "@/wizard";

// The app home (`/`). On a fresh instance the operator lands in the first-run wizard
// instead of an empty Channels page (config-design §6: until `setup.completed` is set,
// `/` routes to the wizard). isSetupCompleted fails open, so an ordinary member is never
// diverted into operator-only setup.
const Route = createFileRoute("/_authed/")({
  beforeLoad: async ({ context }) => {
    const completed = await isSetupCompleted(context.queryClient);
    throw redirect({ to: completed ? "/channels" : "/wizard" });
  },
});

export { Route };
