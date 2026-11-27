import { ArtworkFrame, FileName, Surface, Tabs } from "@loomarr/design-system";
import { libraryFileNames, librarySections } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

type Section = (typeof librarySections)[number]["value"];

// The Library's shared parts on a phone and a TV: underline tabs, 2:3 poster frames, and file names
// that React Native shortens in the middle.
const LibraryParts = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [section, setSection] = useState<Section>("all");
  return (
    <Surface backgroundColor="transparent" borderWidth={0} gap="section">
      <Tabs
        appearance="underline"
        density={density}
        label="Library sections"
        onValueChange={setSection}
        options={librarySections}
        value={section}
      />
      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap="control">
        {(["ready", "missing", "loading"] as const).map((state) => (
          <Surface backgroundColor="transparent" borderWidth={0} flex={1} key={state}>
            <ArtworkFrame borderRadius="control" density={density} ratio="2:3" state={state} width="100%">
              <Surface backgroundColor="stateInfo" borderWidth={0} height="100%" width="100%" />
            </ArtworkFrame>
          </Surface>
        ))}
      </Surface>
      <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
        {Object.values(libraryFileNames).map((name) => (
          <FileName density={density} key={name} name={name} />
        ))}
      </Surface>
    </Surface>
  );
};

const meta = {
  title: "Loomarr Foundations/Library parts",
  component: LibraryParts,
} satisfies Meta<typeof LibraryParts>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
