import { createFileRoute } from "@tanstack/react-router";
import { Placeholder } from "@/components/loomarr";

const SettingsScreen = () => (
  <Placeholder title="Settings" hint="Connections, Channels & Playback, AI, Users, Advanced." />
);

const Route = createFileRoute("/_authed/settings")({
  component: SettingsScreen,
});

export { Route };
