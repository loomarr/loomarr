import { Surface } from "@loomarr/design-system";
import { StatePanel } from "@loomarr/ui";
import { useNavigate } from "@tanstack/react-router";
import { useAuth } from "../use-auth";
import type { AdminOnlyProps } from "./admin-only.type";

// AdminOnly — the one answer a member gets on an admin page opened by URL (#1977). Members have
// no nav path to these pages, so they arrive from a shared link or a typed address. The server
// still refuses every admin read and write (§11, §19). This only replaces what used to show, a
// bare "Forbidden", a silent bounce or a page of editors that 403 on save, with the design
// system's permission state and one way out.
//
// ⚠ The children are NOT rendered for a member, so the admin page's own queries never fire.
const AdminOnly = ({ children }: AdminOnlyProps) => {
  const { isAdmin, isLoading } = useAuth();
  const navigate = useNavigate();
  if (isLoading) return null;
  if (isAdmin) return children;
  return (
    <Surface backgroundColor="$transparent" borderWidth={0} padding="$section">
      <StatePanel
        kind="permission"
        title="Admins only"
        description="Only an admin can open this page. Ask one if you need something changed here."
        action={{ label: "Go to Home", onPress: () => void navigate({ to: "/dashboard" }) }}
      />
    </Surface>
  );
};

export { AdminOnly };
