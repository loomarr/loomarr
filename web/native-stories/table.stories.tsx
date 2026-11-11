import {
  Screen,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  Text,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";

const rows = [
  ["Scan libraries", "Every 6 hours", "2 hours ago"],
  ["Refresh guide data", "Every 12 hours", "9 hours ago"],
] as const;

const NativeTableWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => (
  <Screen density={density} gap="section">
    <Text density={density} textRole="title">
      Table
    </Text>
    <Table caption="Scheduled tasks" density={density}>
      <TableHeader>
        <TableRow>
          <TableHeaderCell>Task</TableHeaderCell>
          <TableHeaderCell>Frequency</TableHeaderCell>
          <TableHeaderCell>Last run</TableHeaderCell>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map(([name, frequency, last]) => (
          <TableRow key={name}>
            <TableCell>{name}</TableCell>
            <TableCell>{frequency}</TableCell>
            <TableCell>{last}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </Screen>
);

const meta = {
  title: "Loomarr Foundations/Table",
  component: NativeTableWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeTableWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };

export default meta;
export { Touch, Tv };
