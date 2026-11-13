import {
  Action,
  Field,
  Screen,
  Stack,
  type StackAlign,
  type StackGap,
  Surface,
  Text,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// Stack — layout only. It draws nothing: the boxes below are Surfaces so the arrangement can be
// seen. Gap comes from the spacing tokens (inline 8, control 12, section 24, screen 32).
const gaps: readonly StackGap[] = ["none", "inline", "control", "section", "screen"];
const aligns: readonly StackAlign[] = ["start", "center", "end", "stretch"];

const wrapping = [
  "Drama",
  "Comedy",
  "Sci-fi",
  "Animation",
  "Documentary",
  "Kids",
  "News",
  "Sport",
  "Music",
  "Mystery",
  "Western",
  "Reality",
] as const;

const Tile = ({ children, tall = false }: { children: string; tall?: boolean }) => (
  <Surface padding="control" paddingVertical={tall ? "section" : "control"}>
    <Text textRole="label">{children}</Text>
  </Surface>
);

const Workshop = () => (
  <Screen gap="section">
    <Surface gap="control" padding="section">
      <Text textRole="title">Gap, down a column</Text>
      <Stack direction="row" gap="section" wrap>
        {gaps.map((gap) => (
          <Stack gap="inline" key={gap}>
            <Text textRole="metadata" tone="secondary">
              {gap}
            </Text>
            <Stack gap={gap}>
              <Tile>One</Tile>
              <Tile>Two</Tile>
              <Tile>Three</Tile>
            </Stack>
          </Stack>
        ))}
      </Stack>
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Align, across a row</Text>
      {aligns.map((align) => (
        <Stack gap="inline" key={align}>
          <Text textRole="metadata" tone="secondary">
            {align}
          </Text>
          <Stack align={align} direction="row" gap="control">
            <Tile>Short</Tile>
            <Tile tall>Tall</Tile>
            <Tile>Short</Tile>
          </Stack>
        </Stack>
      ))}
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Justify and wrap</Text>
      <Stack align="center" direction="row" gap="control" justify="between">
        <Text textRole="body">Title at the start</Text>
        <Action onPress={() => undefined} variant="ghost">
          Action at the end
        </Action>
      </Stack>
      <Stack direction="row" gap="inline" justify="end">
        <Action onPress={() => undefined} variant="outline">
          Cancel
        </Action>
        <Action onPress={() => undefined}>Save</Action>
      </Stack>
      <Stack direction="row" gap="inline" wrap>
        {wrapping.map((name) => (
          <Tile key={name}>{name}</Tile>
        ))}
      </Stack>
    </Surface>
  </Screen>
);

// `render="form"`: the Stack is the <form>, so its arrangement and its attributes live on one element.
// `noValidate` leaves validation to the app's own messages; Enter in the field submits.
const FormWorkshop = () => {
  const [name, setName] = useState("");
  const [sent, setSent] = useState<string | null>(null);
  return (
    <Screen gap="section">
      <Surface padding="section">
        <Stack
          aria-label="Name a channel"
          gap="control"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            setSent(name);
          }}
          render="form"
        >
          <Text textRole="title">Name a channel</Text>
          <Field label="Channel name" onChangeText={setName} placeholder="Saturday Cartoons" value={name} />
          <Stack direction="row" gap="inline" justify="end">
            <Action submit>Save</Action>
          </Stack>
          {sent === null ? null : <Text textRole="body">{`Saved: ${sent}`}</Text>}
        </Stack>
      </Surface>
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Stack",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };
const AsForm: Story = { render: () => <FormWorkshop /> };

export default meta;
export { AsForm, Default, Light };
