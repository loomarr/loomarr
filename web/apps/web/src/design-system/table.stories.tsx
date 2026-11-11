import {
  Action,
  Screen,
  StatusWord,
  Surface,
  Table,
  TableBody,
  TableCell,
  TableHeader,
  TableHeaderCell,
  TableRow,
  Text,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// Table — the Help doc table of the web mock (a bordered box, a raised header row of small tracked
// mono labels, a quiet rule between rows), as a real <table> on web and a flex grid on native.
const symptoms = [
  ["No picture on a channel", "Channels, then the channel's Filler tab"],
  ["Guide is empty", "Settings, Libraries: has the scan finished?"],
  ["A scan never finishes", "Diagnostics, Startup report"],
] as const;

const checks = [
  ["Database", "ok", "2 minutes ago", "Reachable, migrations current"],
  ["Media server", "attention", "2 minutes ago", "The address did not answer"],
  ["Tuner", "notSetUp", "Never", "No tuner configured"],
] as const;

const tasks = [
  ["Scan libraries", "Every 6 hours", "2 hours ago", "in 4 hours", "Looks for new and changed titles."],
  ["Refresh guide data", "Every 12 hours", "9 hours ago", "in 3 hours", "Rebuilds the next days of lineup."],
  ["Prune logs", "Daily at 03:00", "—", "Paused", "Drops log lines past their retention."],
] as const;

const tokens = [
  ["control", "12"],
  ["inline", "8"],
  ["section", "24"],
] as const;

const Workshop = () => {
  const [open, setOpen] = useState<string | null>("Scan libraries");
  return (
    <Screen gap="section">
      <Surface gap="control" padding="section">
        <Text textRole="title">Plain</Text>
        <Table caption="Symptoms and where to look">
          <TableHeader>
            <TableRow>
              <TableHeaderCell width={240}>Symptom</TableHeaderCell>
              <TableHeaderCell>Where to look</TableHeaderCell>
            </TableRow>
          </TableHeader>
          <TableBody>
            {symptoms.map(([symptom, where]) => (
              <TableRow key={symptom}>
                <TableCell mono verticalAlign="top" width={240}>
                  {symptom}
                </TableCell>
                <TableCell verticalAlign="top">{where}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Surface>

      <Surface gap="control" padding="section">
        <Text textRole="title">Status words, sideways scroll under 44rem</Text>
        <Table caption="Current health checks" minWidth={704}>
          <TableHeader>
            <TableRow>
              <TableHeaderCell>Check</TableHeaderCell>
              <TableHeaderCell>Status</TableHeaderCell>
              <TableHeaderCell>Last checked</TableHeaderCell>
              <TableHeaderCell>Detail</TableHeaderCell>
            </TableRow>
          </TableHeader>
          <TableBody>
            {checks.map(([name, kind, when, detail]) => (
              <TableRow key={name}>
                <TableCell>{name}</TableCell>
                <TableCell>
                  <StatusWord kind={kind} />
                </TableCell>
                <TableCell>{when}</TableCell>
                <TableCell>{detail}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Surface>

      <Surface gap="control" padding="section">
        <Text textRole="title">Actions, an opened detail row, a dimmed row</Text>
        <Table caption="Scheduled tasks">
          <TableHeader>
            <TableRow>
              <TableHeaderCell>Task</TableHeaderCell>
              <TableHeaderCell width={160}>Frequency</TableHeaderCell>
              <TableHeaderCell width={120}>Last run</TableHeaderCell>
              <TableHeaderCell width={120}>Next run</TableHeaderCell>
              <TableHeaderCell align="end" width={56}>
                {""}
              </TableHeaderCell>
            </TableRow>
          </TableHeader>
          <TableBody>
            {tasks.map(([name, frequency, last, next, description]) => (
              <TableRowGroup
                description={description}
                dimmed={next === "Paused"}
                frequency={frequency}
                key={name}
                last={last}
                name={name}
                next={next}
                onToggle={() => setOpen(open === name ? null : name)}
                open={open === name}
              />
            ))}
          </TableBody>
        </Table>
      </Surface>

      <Surface gap="control" padding="section">
        <Text textRole="title">No header, no frame</Text>
        <Table caption="Spacing tokens" framed={false}>
          <TableBody>
            {tokens.map(([name, value]) => (
              <TableRow key={name}>
                <TableCell mono>{name}</TableCell>
                <TableCell mono>{value}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Surface>
    </Screen>
  );
};

const TableRowGroup = ({
  description,
  dimmed,
  frequency,
  last,
  name,
  next,
  onToggle,
  open,
}: {
  description: string;
  dimmed: boolean;
  frequency: string;
  last: string;
  name: string;
  next: string;
  onToggle: () => void;
  open: boolean;
}) => (
  <>
    <TableRow dimmed={dimmed}>
      <TableCell>{name}</TableCell>
      <TableCell>{frequency}</TableCell>
      <TableCell>{last}</TableCell>
      <TableCell>{next}</TableCell>
      <TableCell align="end">
        <Action icon={open ? "rowCollapse" : "rowExpand"} layout="icon" onPress={onToggle} variant="ghost">
          {`${open ? "Collapse" : "Expand"} ${name}`}
        </Action>
      </TableCell>
    </TableRow>
    {open ? (
      <TableRow tone="raised">
        <TableCell colSpan={5}>{description}</TableCell>
      </TableRow>
    ) : null}
  </>
);

const meta = {
  title: "Loomarr Components/Table",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Default, Light };
