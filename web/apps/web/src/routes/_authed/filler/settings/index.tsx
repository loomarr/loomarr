import { createFileRoute } from "@tanstack/react-router";
import { warmFillerPage } from "@/filler/filler-loader";
import { FillerPage } from "@/filler/filler-page";

// The index is a list of links; the page's status line is all it reads.
const Route = createFileRoute("/_authed/filler/settings/")({
  loader: ({ context: { queryClient } }) => warmFillerPage(queryClient),
  component: () => <FillerPage tab="settings" />,
});

export { Route };
