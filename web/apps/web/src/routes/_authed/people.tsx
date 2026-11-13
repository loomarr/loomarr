import * as invitationsApi from "@loomarr/api/endpoints/invitations";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as usersApi from "@loomarr/api/endpoints/users";
import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
import { viewerIsAdmin } from "@/auth/me-query";
import { useDocumentTitle } from "@/lib/use-document-title";
import { UsersPage } from "@/people/users-page";

// People is admin-only. The server enforces it (every /v1/users route 403s for a member —
// §11, §19); AdminOnly gives a member who follows a link the shared no-access state instead
// of a page of failed requests.
const PeopleScreen = () => {
  useDocumentTitle("People");
  return (
    <AdminOnly>
      <UsersPage />
    </AdminOnly>
  );
};

const Route = createFileRoute("/_authed/people")({
  // The people and invitations the page lists, and the settings list that says whether a media
  // server can be imported from. Admin-only, so a member's request is never sent. The page shows
  // its own state for each, so they are warmed.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.prefetchQuery(usersApi.getListUsersQueryOptions()),
      queryClient.prefetchQuery(invitationsApi.getListInvitationsQueryOptions()),
      queryClient.prefetchQuery(settingsApi.getSettingsListQueryOptions()),
    ]);
  },
  component: PeopleScreen,
});

export { Route };
