import { FileName } from "@loomarr/design-system";
import { libraryFileNames } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";

// FileName: a file name in the data face on one line. When it does not fit, the middle gives way
// and the end of the name and its extension stay; the whole name is read by assistive tech and
// shown on hover. Each story sets every fixture shape in a column of the story's width.
const FileNames = ({ strong = false, width }: { strong?: boolean; width: number }) => (
  <div style={{ display: "flex", flexDirection: "column", gap: 12, maxWidth: width, padding: 16 }}>
    {Object.values(libraryFileNames).map((name) => (
      <FileName key={name} name={name} strong={strong} />
    ))}
  </div>
);

const meta = {
  title: "Loomarr Foundations/FileName",
  component: FileNames,
  args: { strong: false, width: 720 },
} satisfies Meta<typeof FileNames>;

type Story = StoryObj<typeof meta>;

const Wide: Story = {};
const Strong: Story = { args: { strong: true } };
// A phone row: most names shorten in the middle at 390.
const Phone: Story = { args: { width: 390 } };
// A narrow column beside a poster: even the tail nearly fills it.
const Narrow: Story = { args: { width: 220 } };
const Light: Story = { args: { width: 390 }, globals: { theme: "light" } };

export default meta;
export { Light, Narrow, Phone, Strong, Wide };
