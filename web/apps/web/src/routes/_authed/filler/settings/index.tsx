import * as fillerApi from "@loomarr/api/endpoints/filler";
import { createFileRoute } from "@tanstack/react-router";
import { FillerPage } from "@/filler/filler-page";

// The index is a list of links; the page's status line is all it waits for, so it is warmed.
const Route = createFileRoute("/_authed/filler/settings/")({
  loader: ({ context: { queryClient } }) => queryClient.prefetchQuery(fillerApi.getFillerWatchQueryOptions()),
  component: () => <FillerPage tab="settings" />,
});

export { Route };
