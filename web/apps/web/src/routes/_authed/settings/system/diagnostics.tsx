import { createFileRoute } from "@tanstack/react-router";
import { StartupReportPage } from "@/diagnostics";

const Route = createFileRoute("/_authed/settings/system/diagnostics")({ component: StartupReportPage });

export { Route };
