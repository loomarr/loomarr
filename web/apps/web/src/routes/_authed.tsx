import { ApiError, authApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Outlet, redirect, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { meQueryOptions, needsBootstrap, useAuth } from "@/auth";
import { AppShell, RestartOverlay } from "@/components/loomarr";
import { RestartWatchProvider, useRestartWatchContext } from "@/dashboard/restart-watch-provider";
import { LoomarrEventsProvider } from "@/events";
import { CommandPalette, useCommandShortcut } from "@/palette";

// The authenticated app layout + session gate (§11). beforeLoad ensures the me query
// (shared meQueryOptions) before any child renders — a 401 throws a redirect to /login
// with the intended href remembered, so there's no guard-spinner flash. The component
// is the AppShell frame wrapped in LoomarrEventsProvider — ONE SSE stream for the app's
// lifetime, which both invalidates caches (keeping every surface live, §12) and fans the
// frames out to screens that need the payload itself, like the Suggest workspace's live
// phases. Identity comes from useAuth (real name, admin-only nav), and logout clears the
// query cache and drops back to /login.
// The shell split in two so the overlay can read the restart state the provider holds —
// a provider cannot consume its own context.
const AuthedLayout = () => (
  <RestartWatchProvider>
    <AuthedFrame />
  </RestartWatchProvider>
);

const AuthedFrame = () => {
  const [commandOpen, setCommandOpen] = useState(false);
  useCommandShortcut(setCommandOpen);
  const { user, isAdmin } = useAuth();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const restartWatch = useRestartWatchContext();

  const logout = authApi.useLogout({
    mutation: {
      onSuccess: () => {
        queryClient.clear();
        navigate({ to: "/login" });
      },
    },
  });

  return (
    <LoomarrEventsProvider>
      <AppShell
        isAdmin={isAdmin}
        userName={user?.name ?? "…"}
        onOpenCommand={() => setCommandOpen(true)}
        onLogout={() => logout.mutate()}
      >
        <Outlet />
      </AppShell>
      <CommandPalette open={commandOpen} onOpenChange={setCommandOpen} />
      {/* ⚠ App-wide, not per page. The restart control lives on the Dashboard, but the
          operator can navigate away the instant they click it — and every page is equally
          dead while the server rebuilds. */}
      <RestartOverlay
        restarting={restartWatch.restarting}
        justCameBack={restartWatch.justCameBack}
        failed={restartWatch.failed}
      />
    </LoomarrEventsProvider>
  );
};

const Route = createFileRoute("/_authed")({
  beforeLoad: async ({ context, location }) => {
    try {
      await context.queryClient.ensureQueryData(meQueryOptions());
    } catch (err) {
      // ⚠ **Only a 401 means "signed out".** This used to be a bare `catch {}`, and with
      // `meQueryOptions` setting retry:false ("a 401 is a definite answer") NOTHING inspected the
      // status — so a network blip, a 500, a proxy hiccup or this app's own self-restart bounced a
      // user holding a perfectly valid cookie to /login?redirect=… . That is not an edge case
      // here: RestartWatchProvider/RestartOverlay are wired into this very layout because the
      // operator can restart the server from the Dashboard, and every such restart hit this path.
      //
      // A non-401 is rethrown so it surfaces as the failure it is — the restart overlay and the
      // error boundary exist to handle exactly that, and neither can if the guard has already
      // reported it as a logged-out session.
      if (!(err instanceof ApiError) || err.status !== 401) throw err;

      // No session. Before sending them to a login form, ask whether this install
      // even HAS accounts yet (§7/§13): on a fresh one, /login is a door with no key
      // — nothing on the page says the install is unclaimed, so the owner is simply
      // stuck. Only reached on the 401 path, so a claimed install pays nothing.
      if (await needsBootstrap(context.queryClient)) throw redirect({ to: "/wizard" });
      // `location.href` is path+search+hash with no origin, so it is same-app by construction;
      // /login's validateSearch validates it again on arrival regardless.
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
  },
  component: AuthedLayout,
});

export { Route };
