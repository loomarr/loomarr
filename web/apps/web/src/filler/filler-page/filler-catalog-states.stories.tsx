import type { ClipDTO } from "@loomarr/api";
import { Stack, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ClipRow } from "@/components/loomarr/filler/clip-row";
import { widthFrame } from "@/test/story-utils";

const clips: ClipDTO[] = Array.from({ length: 60 }, (_, index) => ({
  hash: `catalog-${index.toString().padStart(3, "0")}`,
  name: `${index % 3 === 0 ? "Soda" : index % 3 === 1 ? "Toy" : "Station"} spot ${index + 1}`,
  role: index % 3 === 2 ? "station_id" : "commercial",
  era: 1980 + (index % 5) * 10,
  audience: index % 4 === 0 ? "kids" : "general",
  category: index % 3 === 0 ? "food_drink" : index % 3 === 1 ? "toys" : "station_id",
  durationMs: 15_000 + (index % 4) * 5_000,
  source: "automatic",
  playCount: index * 2,
  playsCounted: true,
  tagged: true,
  needsDetails: false,
}));

const LargeCatalogState = () => (
  <section aria-labelledby="large-catalog-heading">
    <Stack marginBottom={12}>
      <Text as="h2" id="large-catalog-heading" textRole="headline" strong>
        60 clips
      </Text>
      <Text as="p" textRole="compact" tone="secondary">
        Showing 1–60 of 1,240
      </Text>
    </Stack>
    <Stack borderColor="$borderDecorative" borderRadius={8} borderWidth={1} overflow="hidden">
      <Stack marginBottom={-1}>
        {clips.map((clip) => (
          <ClipRow key={clip.hash} clip={clip} />
        ))}
      </Stack>
    </Stack>
  </section>
);

const meta = {
  title: "Filler/CatalogStates",
  component: LargeCatalogState,
  decorators: [widthFrame(960)],
} satisfies Meta<typeof LargeCatalogState>;

export default meta;
type Story = StoryObj<typeof meta>;

export const LargeCatalog: Story = {};
