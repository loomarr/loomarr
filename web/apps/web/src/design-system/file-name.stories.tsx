import {
  FileName,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  Text,
} from "@loomarr/design-system";
import { libraryFileNames } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";

// FileName: a file name in the data face on one line. When it does not fit, the middle gives way
// and the end of the name and its extension stay; the whole name shows on hover. The cut prefers a
// word start; a single last word longer than the tail may be cut, at a character boundary.
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
// A narrow column beside a poster.
const Narrow: Story = { args: { width: 220 } };
// Narrower than most tails: the head is gone and the tail gives way with its own ellipsis, inside
// its box.
const NarrowerThanTail: Story = { args: { width: 96 } };
const Light: Story = { args: { width: 390 }, globals: { theme: "light" } };

// A right-to-left name runs right to left, its ellipsis beside its tail.
const RightToLeft: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: 12, padding: 16 }}>
      {[720, 260].map((width) => (
        <div key={width} style={{ maxWidth: width }}>
          <FileName name={libraryFileNames.rightToLeft} />
        </div>
      ))}
    </div>
  ),
};

// The places it sits: a flex row beside an action, a grid cell, a table cell, and inside a sentence.
const InLayouts: Story = {
  render: () => (
    <div style={{ display: "flex", flexDirection: "column", gap: 24, maxWidth: 390, padding: 16 }}>
      <div style={{ alignItems: "center", display: "flex", gap: 12 }}>
        <div style={{ flex: 1, minWidth: 0 }}>
          <FileName name={libraryFileNames.long} strong />
        </div>
        <Text textRole="label">Fix</Text>
      </div>
      <div style={{ display: "grid", gap: 12, gridTemplateColumns: "minmax(0, 1fr) minmax(0, 1fr)" }}>
        <FileName name={libraryFileNames.spaced} />
        <FileName name={libraryFileNames.underscored} />
      </div>
      <Table caption="Files">
        <TableHeader>
          <TableRow>
            <TableHeaderCell width={240}>File</TableHeaderCell>
            <TableHeaderCell align="end" width={72}>
              Size
            </TableHeaderCell>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell>
              <FileName layout="fill" name={libraryFileNames.long} />
            </TableCell>
            <TableCell align="end">8.1 GB</TableCell>
          </TableRow>
        </TableBody>
      </Table>
      <Text as="p" textRole="body">
        Loomarr could not read <FileName name={libraryFileNames.longLastWord} /> and set it aside.
      </Text>
    </div>
  ),
};

export default meta;
export { InLayouts, Light, Narrow, NarrowerThanTail, Phone, RightToLeft, Strong, Wide };
