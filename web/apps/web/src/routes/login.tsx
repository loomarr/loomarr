import { authApi } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute, redirect, useRouter } from "@tanstack/react-router";
import { meQueryOptions, needsBootstrap } from "@/auth";
import { LoginForm, LoginShell } from "@/components/loomarr";
import { useDocumentTitle } from "@/lib";

// Login — the public sign-in screen (§11, §13). An idle surface (dark broadcast frame).
// beforeLoad bounces an already-signed-in visitor to where they were headed; the
// component wires useLogin and, on success, refreshes identity (the app's source of
// truth) and returns them there — or the Channels home. The `redirect` search param is
// typed (replaces react-router's location.state) and set by the _authed guard.
interface LoginSearch {
  redirect?: string;
}

const LoginScreen = () => {
  useDocumentTitle("Sign in");
  const router = useRouter();
  const queryClient = useQueryClient();
  const { redirect: dest } = Route.useSearch();

  const login = authApi.useLogin({
    mutation: {
      onSuccess: async () => {
        await queryClient.invalidateQueries({ queryKey: authApi.getMeQueryKey() });
        router.history.push(dest ?? "/channels");
      },
    },
  });

  return (
    <LoginShell>
      <LoginForm
        onSubmit={(values) => login.mutate({ data: values })}
        isPending={login.isPending}
        error={login.error}
      />
    </LoginShell>
  );
};

const Route = createFileRoute("/login")({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    redirect: typeof search.redirect === "string" ? search.redirect : undefined,
  }),
  beforeLoad: async ({ context, search }) => {
    try {
      await context.queryClient.ensureQueryData(meQueryOptions());
    } catch {
      // Not signed in — but on an UNCLAIMED install there is no credential that
      // could work, so showing the form would strand the owner (§7/§13). Guarding
      // here as well as in _authed covers the operator who navigates to /login
      // directly, or who bookmarked it before bootstrapping.
      if (await needsBootstrap(context.queryClient)) throw redirect({ to: "/wizard" });
      return; // signed out on a claimed install → show the form
    }
    throw redirect({ href: search.redirect ?? "/channels" });
  },
  component: LoginScreen,
});

export { Route };
