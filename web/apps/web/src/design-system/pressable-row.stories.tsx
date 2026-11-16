import { Action, PressableRow, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// The whole-row button (#2147): a list of things that open or pick one.
const rows = [
  { meta: "0:30 · from Archive", name: "Cereal ad, 1989", status: "Ready" },
  { meta: "0:15 · from Archive", name: "Toy commercial, 1992", status: "Checking" },
  { meta: "1:00 · from Local", name: "Station ident", status: "Ready" },
] as const;

const links = ["Guide", "Channels", "Library"] as const;

const List = () => {
  const [open, setOpen] = useState<string>(rows[1].name);
  return (
    <Surface backgroundColor="transparent" borderColor="$borderDecorative" borderWidth={1} borderRadius={12}>
      {rows.map((row, index) => (
        <PressableRow
          divided={index < rows.length - 1}
          key={row.name}
          onPress={() => setOpen(row.name)}
          selected={open === row.name}
        >
          <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap={2}>
            <Text textRole="label">{row.name}</Text>
            <Text textRole="hint" tone="muted">
              {row.meta}
            </Text>
          </Surface>
          <Text textRole="hint" tone="muted">
            {row.status}
          </Text>
        </PressableRow>
      ))}
    </Surface>
  );
};

const PressableRowWorkshop = () => (
  <Screen gap="section">
    <Surface gap="control" padding="section">
      <Text textRole="title">Rows that open an item</Text>
      <List />
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Rows that are links</Text>
      <Surface
        backgroundColor="transparent"
        borderColor="$borderDecorative"
        borderWidth={1}
        borderRadius={12}
      >
        {links.map((link, index) => (
          <PressableRow
            divided={index < links.length - 1}
            key={link}
            render={<a href={`#${link.toLowerCase()}`} />}
            selected={index === 1}
          >
            <Text textRole="label">{link}</Text>
          </PressableRow>
        ))}
      </Surface>
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Disabled</Text>
      <PressableRow disabled onPress={() => undefined}>
        <Text textRole="label">Waiting for the scan</Text>
      </PressableRow>
    </Surface>
  </Screen>
);

const meta = {
  title: "Loomarr Components/PressableRow",
  component: PressableRowWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof PressableRowWorkshop>;

// A row that stands apart as a card (a channel in a drawer): the current one is stronger, hover
// lifts the rest, and a pin sits over its corner.
const channels = [
  { name: "Westerns", now: "A western", number: "07" },
  { name: "Cartoons", now: "A cartoon", number: "12" },
  { name: "Movies", now: "A film", number: "21" },
] as const;

const BorderedWorkshop = () => {
  const [tuned, setTuned] = useState<string>(channels[1].name);
  const [pinned, setPinned] = useState<string>(channels[0].name);
  return (
    <Screen gap="section">
      <Surface gap="control" padding="section">
        <Text textRole="title">Bordered rows</Text>
        <Surface backgroundColor="transparent" borderWidth={0} gap={7} padding={0}>
          {channels.map((channel) => (
            <Surface
              backgroundColor="transparent"
              borderWidth={0}
              key={channel.name}
              padding={0}
              position="relative"
            >
              <PressableRow bordered onPress={() => setTuned(channel.name)} selected={tuned === channel.name}>
                <Surface
                  backgroundColor="transparent"
                  borderWidth={0}
                  flexDirection="row"
                  gap={9}
                  padding={0}
                >
                  <Text textRole="mono" tone="secondary">
                    {channel.number}
                  </Text>
                  <Text strong textRole="cardLabel" tone="primary">
                    {channel.name}
                  </Text>
                </Surface>
                <Text textRole="cardMeta" tone="secondary">
                  {`▸ ${channel.now}`}
                </Text>
              </PressableRow>
              <Surface
                backgroundColor="transparent"
                borderWidth={0}
                padding={0}
                position="absolute"
                right={8}
                top={8}
              >
                <Action
                  aria-pressed={pinned === channel.name}
                  compact
                  icon={pinned === channel.name ? "starFilled" : "star"}
                  layout="icon"
                  onPress={() => setPinned(channel.name)}
                  selected={pinned === channel.name}
                  variant="ghost"
                >
                  {`Favorite ${channel.name}`}
                </Action>
              </Surface>
            </Surface>
          ))}
        </Surface>
      </Surface>
    </Screen>
  );
};

type Story = StoryObj<typeof meta>;
const Workshop: Story = {};
const Bordered: Story = { render: () => <BorderedWorkshop /> };

export default meta;
export { Bordered, Workshop };
