import * as usersApi from "@loomarr/api/endpoints/users";
import { createFileRoute } from "@tanstack/react-router";
import { AccountPage } from "@/account/account-page";
import { meQueryOptions } from "@/auth/me-query";

const Route = createFileRoute("/_authed/account")({
  // The viewer's own sessions, so the list is there with the page rather than "Loading sessions…".
  // The page shows its own state for a failed read, so it is warmed.
  loader: async ({ context: { queryClient } }) => {
    const me = await queryClient.ensureQueryData(meQueryOptions());
    if (me.status !== 200) return;
    await queryClient.prefetchQuery(usersApi.getListUserSessionsQueryOptions(me.data.id));
  },
  component: AccountPage,
});

export { Route };
