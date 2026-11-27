import { createFileRoute, redirect } from "@tanstack/react-router";
import { meQueryOptions } from "@/auth/me-query";
import { openFillerPage, warmDefectReport } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";

// Quality — the defect floor's labelling queue (#2034). Admin-only, as the report and its labels
// are: they are the maintainer's calibration of this install's floor.
const QualityScreen = () => <FillerPage tab="quality" />;

const Route = createFileRoute("/_authed/filler/quality")({
  beforeLoad: async ({ context }) => {
    const me = await context.queryClient.ensureQueryData(meQueryOptions());
    if (me.status !== 200 || me.data.role !== "admin") {
      throw redirect({ to: "/filler/library" });
    }
  },
  // beforeLoad has already sent a member away. The queue opens on the first unlabelled clip. The
  // report is warmed: the queue has its own error and loading panels, which Skip reaches too.
  loader: async ({ context: { queryClient } }) => {
    await Promise.all([openFillerPage(queryClient), warmDefectReport(queryClient)]);
  },
  component: QualityScreen,
});

export { Route };
