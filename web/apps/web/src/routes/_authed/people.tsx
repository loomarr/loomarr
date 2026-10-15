import { createFileRoute } from "@tanstack/react-router";
import { AdminOnly } from "@/auth/admin-only";
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
  component: PeopleScreen,
});

export { Route };
