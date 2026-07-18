import { createFileRoute } from "@tanstack/react-router";
import { Placeholder } from "@/components/loomarr";

const FillerScreen = () => (
  <Placeholder
    title="Filler"
    hint="No clips yet — drop files in the filler folder or point MeTube at a playlist."
  />
);

const Route = createFileRoute("/_authed/filler")({
  component: FillerScreen,
});

export { Route };
