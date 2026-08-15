import { authApi, setupApi } from "@loomarr/api";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, redirect, useRouter } from "@tanstack/react-router";
import { meQueryOptions, readSetupState, safeRedirectPath } from "@/auth";
import { LoginForm, LoginShell } from "@/components/loomarr";
import { useDocumentTitle } from "@/lib";

// Login — the public sign-in screen (§11, §13). An idle surface (dark broadcast frame).
// beforeLoad bounces an already-signed-in visitor to where they were headed; the
// component wires useLogin and, on success, refreshes identity (the app's source of
// truth) and returns them there — or the Channels home. The `redirect` search param is
// typed (replaces react-router's location.state) and set by the _authed guard.
interface LoginSearch {
  redirect?: string;
  // A reason code from a refused SSO round trip. A CODE, never a message — the callback
  // redirects with one so no server text can be reflected into this page (§11, V8).
  sso?: string;
}

const LoginScreen = () => {
  useDocumentTitle("Sign in");
  const router = useRouter();
  const queryClient = useQueryClient();
  const { redirect: dest, sso: ssoError } = Route.useSearch();

  // ⚠ REPLACE, not push. Pushing leaves /login?redirect=… in the back stack, so the first Back
  // press after signing in lands on the login route, whose guard immediately redirects forward
  // again — Back appears broken rather than slow.
  //
  // ⚠ Re-validated here even though validateSearch already sanitised it, which is the posture
  // ssoroutes.go argues for at its own second call site ("Re-validated HERE, not merely at
  // /start"): this is a navigation, and a navigation should not depend on a caller upstream
  // having been careful.
  const landing = async () => {
    await queryClient.invalidateQueries({ queryKey: authApi.getMeQueryKey() });
    router.history.replace(safeRedirectPath(dest) ?? "/guide");
  };

  const login = authApi.useLogin({ mutation: { onSuccess: landing } });

  // Whether the SERVER mounted the dev-login route (§11). Reusing the same
  // GET /v1/setup/state the bootstrap guard already fetches, so this costs no extra
  // request — and the answer comes from what the server actually registered rather
  // than from a build-time constant that could ship enabled.
  const setupState = useQuery(setupApi.getSetupStateQueryOptions({ query: { retry: false } }));
  const devLoginOffered = setupState.data?.status === 200 && setupState.data.data.devLogin === true;
  // Same request, same posture as devLogin: the SERVER says whether SSO is configured, so the
  // button can never appear pointing at routes that are not mounted.
  const ssoOffered = setupState.data?.status === 200 && setupState.data.data.sso === true;

  // The endpoint is deliberately ABSENT from api/openapi.yaml (a dev bypass is not part
  // of the product contract), so there is no generated hook for it — this is the one
  // sanctioned hand-written call. It is also why the button only renders when the
  // server said the route exists: nothing here would type-check that for us.
  const devLogin = async () => {
    const res = await fetch("/v1/auth/dev-login", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-Loomarr-Csrf": "1" },
    });
    if (res.ok) await landing();
  };

  return (
    <LoginShell>
      <LoginForm
        onSubmit={(values) => login.mutate({ data: values })}
        isPending={login.isPending}
        error={login.error}
        {...(devLoginOffered ? { onDevLogin: () => void devLogin() } : {})}
        {...(ssoOffered
          ? {
              // A full navigation, not a fetch: the flow is a redirect to the provider and
              // back, which an XHR cannot follow. `next` carries the deep link the guard
              // remembered, and the server validates it as a same-app path.
              onSso: () => {
                const next = dest ? `?next=${encodeURIComponent(dest)}` : "";
                window.location.assign(`/v1/auth/sso/start${next}`);
              },
            }
          : {})}
        {...(ssoError ? { ssoError } : {})}
      />
    </LoginShell>
  );
};

const Route = createFileRoute("/login")({
  // ⚠ `redirect` is VALIDATED here, at the emitter, so a hostile value never reaches the
  // component or either navigation (§11). `safeRedirectPath` also subsumes the old
  // typeof-string check: anything that is not a same-app path becomes `undefined`, which is
  // indistinguishable from the param being absent.
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    redirect: safeRedirectPath(search.redirect),
    sso: typeof search.sso === "string" ? search.sso : undefined,
  }),
  beforeLoad: async ({ context, search }) => {
    try {
      await context.queryClient.ensureQueryData(meQueryOptions());
    } catch {
      // Not signed in — but on an UNCLAIMED install there is no credential that
      // could work, so showing the form would strand the owner (§7/§13). Guarding
      // here as well as in _authed covers the operator who navigates to /login
      // directly, or who bookmarked it before bootstrapping.
      const setupState = await readSetupState(context.queryClient);
      if (setupState?.bootstrapped === false) {
        throw redirect({ to: "/wizard" });
      }
      if (setupState?.devLogin === true) {
        const res = await fetch("/v1/auth/dev-login", {
          method: "POST",
          credentials: "same-origin",
          headers: { "X-Loomarr-Csrf": "1" },
        });
        if (res.ok) {
          await context.queryClient.invalidateQueries({ queryKey: authApi.getMeQueryKey() });
          throw redirect({ href: safeRedirectPath(search.redirect) ?? "/guide" });
        }
      }
      return; // signed out on a claimed install → show the form
    }
    // ⚠ The signed-in bounce is the path that was exploitable: the router force-commits a
    // beforeLoad redirect with `replace: true`, so an off-site `href` became a
    // window.location.replace() straight off the app. Re-validated for the same reason `landing`
    // is — this is the second navigation, not the same one.
    throw redirect({ href: safeRedirectPath(search.redirect) ?? "/guide" });
  },
  component: LoginScreen,
});

export { Route };
