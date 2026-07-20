import { authApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Outlet, redirect, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { meQueryOptions, useAuth } from "@/auth";
import { AppShell } from "@/components/loomarr";
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
const AuthedLayout = () => {
  const [commandOpen, setCommandOpen] = useState(false);
  useCommandShortcut(setCommandOpen);
  const { user, isAdmin } = useAuth();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

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
    </LoomarrEventsProvider>
  );
};

const Route = createFileRoute("/_authed")({
  beforeLoad: async ({ context, location }) => {
    try {
      await context.queryClient.ensureQueryData(meQueryOptions());
    } catch {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
  },
  component: AuthedLayout,
});

export { Route };
