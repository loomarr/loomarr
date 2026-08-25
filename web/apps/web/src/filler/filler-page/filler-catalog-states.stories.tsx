import type { ClipDTO } from "@loomarr/api";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ClipRow } from "@/components/loomarr/filler/clip-row";
import { widthFrame } from "@/test/story-utils";

const clips: ClipDTO[] = Array.from({ length: 60 }, (_, index) => ({
  hash: `catalog-${index.toString().padStart(3, "0")}`,
  name: `${index % 3 === 0 ? "Soda" : index % 3 === 1 ? "Toy" : "Station"} spot ${index + 1}`,
  kind: index % 3 === 2 ? "station_id" : "commercial",
  era: 1980 + (index % 5) * 10,
  audience: index % 4 === 0 ? "kids" : "general",
  category: index % 3 === 0 ? "food_drink" : index % 3 === 1 ? "toys" : "station_id",
  durationMs: 15_000 + (index % 4) * 5_000,
  source: "automatic",
  playCount: index * 2,
  playsCounted: true,
  aiTagged: index % 4 === 0,
  tagged: true,
  suggestedEra: 0,
}));

const LargeCatalogState = () => (
  <section aria-labelledby="large-catalog-heading">
    <div className="mb-3">
      <h2 id="large-catalog-heading" className="font-semibold text-lg">
        60 clips
      </h2>
      <p className="text-muted-foreground text-sm">Showing 1–60 of 1,240</p>
    </div>
    <div className="overflow-hidden rounded-lg border border-border">
      {clips.map((clip) => (
        <ClipRow key={clip.hash} clip={clip} />
      ))}
    </div>
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
