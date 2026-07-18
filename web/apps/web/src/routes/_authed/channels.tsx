import { createFileRoute } from "@tanstack/react-router";
import { Placeholder } from "@/components/loomarr";

const ChannelsScreen = () => (
  <Placeholder title="Channels" hint="No channels yet — create your first from an intent." />
);

const Route = createFileRoute("/_authed/channels")({
  component: ChannelsScreen,
});

export { Route };
