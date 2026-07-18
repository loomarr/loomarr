import { authApi } from "@loomarr/api";
import { useLoomarrEvents } from "@loomarr/core";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, Outlet, redirect, useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import { meQueryOptions, useAuth } from "@/auth";
import { AppShell } from "@/components/loomarr";

// The authenticated app layout + session gate (§11). beforeLoad ensures the me query
// (shared meQueryOptions) before any child renders — a 401 throws a redirect to /login
// with the intended href remembered, so there's no guard-spinner flash. The component
// is the AppShell frame + the SSE invalidation stream (core.useLoomarrEvents keeps every
// surface live, §12); identity comes from useAuth (real name, admin-only nav), and
// logout clears the query cache and drops back to /login.
const AuthedLayout = () => {
  const [, setCommandOpen] = useState(false);
  const { user, isAdmin } = useAuth();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  useLoomarrEvents();

  const logout = authApi.useLogout({
    mutation: {
      onSuccess: () => {
        queryClient.clear();
        navigate({ to: "/login" });
      },
    },
  });

  return (
    <AppShell
      isAdmin={isAdmin}
      userName={user?.name ?? "…"}
      onOpenCommand={() => setCommandOpen(true)}
      onLogout={() => logout.mutate()}
    >
      <Outlet />
    </AppShell>
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
