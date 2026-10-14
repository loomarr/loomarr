import { createFileRoute } from "@tanstack/react-router";
import { useAuth } from "@/auth/use-auth";
import { EmptyState } from "@/components/loomarr/feedback/empty-state";
import { useDocumentTitle } from "@/lib/use-document-title";
import { UsersPage } from "@/people/users-page";

// People is admin-only. The server enforces it (every /v1/users route 403s for a member —
// §11, §19); this check exists so a member who follows a link sees an explanation instead
// of a page of failed requests.
const PeopleScreen = () => {
  useDocumentTitle("People");
  const { isAdmin, isLoading } = useAuth();
  if (isLoading) return null;
  if (!isAdmin) {
    return (
      <EmptyState
        title="Admins only"
        description="Managing who can sign in is an admin action. Ask an admin if you need access changed."
      />
    );
  }
  return <UsersPage />;
};

const Route = createFileRoute("/_authed/people")({
  component: PeopleScreen,
});

export { Route };
