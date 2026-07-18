import { createFileRoute } from "@tanstack/react-router";
import { Placeholder } from "@/components/loomarr";

const UsersScreen = () => (
  <Placeholder title="Users" hint="Import Emby/Jellyfin accounts to let them sign in." />
);

const Route = createFileRoute("/_authed/users")({
  component: UsersScreen,
});

export { Route };
