import { createFileRoute } from "@tanstack/react-router";
import { ChannelFiller } from "@/filler";
import { useChannelDetail } from "./-channel-detail-context";

// Filler — admin-only. Keeps its internal sandbox (the live pod preview).
//
// ⚠ A visually-hidden heading here. `ChannelFiller`'s own `CollapsibleSection` title is
// deliberately a `<span>`, not a heading (its header is a toggle button — see
// collapsible-section.tsx), and it already labels the section visibly, so a second VISIBLE
// "Filler" caption right above it would just repeat the same two lines back to back. This is
// its own route now (V-nav-paths) rather than a panel always shown beside the info tab's own
// `<h2>`, so it still needs a real heading for the page to be provably "reachable" — see the
// visible-heading reachability check — without duplicating what's already on screen.
const FillerScreen = () => {
  const { id, channel } = useChannelDetail();
  return (
    <>
      <h2 className="sr-only">Filler</h2>
      <ChannelFiller channelId={id} policy={channel.policy} open />
    </>
  );
};

const Route = createFileRoute("/_authed/channels/$id/filler")({
  component: FillerScreen,
});

export { Route };
