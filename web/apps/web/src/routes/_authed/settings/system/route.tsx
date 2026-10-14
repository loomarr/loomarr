import { createFileRoute, Outlet } from "@tanstack/react-router";

const SystemLayout = () => <Outlet />;

const Route = createFileRoute("/_authed/settings/system")({
  component: SystemLayout,
});

export { Route };
