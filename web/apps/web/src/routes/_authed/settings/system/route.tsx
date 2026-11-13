import { createFileRoute, Outlet } from "@tanstack/react-router";

const SystemLayout = () => <Outlet />;

// no-loader: only renders the System pages' outlet
const Route = createFileRoute("/_authed/settings/system")({
  component: SystemLayout,
});

export { Route };
