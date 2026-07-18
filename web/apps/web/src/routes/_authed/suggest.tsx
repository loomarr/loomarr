import { createFileRoute } from "@tanstack/react-router";
import { Placeholder } from "@/components/loomarr";

const SuggestScreen = () => (
  <Placeholder title="Suggest" hint="Describe a channel and Loomarr grounds a lineup against your library." />
);

const Route = createFileRoute("/_authed/suggest")({
  component: SuggestScreen,
});

export { Route };
