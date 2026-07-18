import { createFileRoute } from "@tanstack/react-router";
import { Placeholder } from "@/components/loomarr";

const HelpScreen = () => <Placeholder title="Help" hint="The embedded docs — searchable, offline." />;

const Route = createFileRoute("/_authed/help")({
  component: HelpScreen,
});

export { Route };
