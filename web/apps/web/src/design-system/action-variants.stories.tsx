import { Action, Badge, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The six legacy Web Button variants plus the web `render` composition escape hatch (#970 PR B
// checkpoint 3), kept in their own file rather than a new row on InteractionWorkshop so this new
// content doesn't move that story's existing visual baselines.
const ActionVariantsWorkshop = () => (
  <Screen gap="section">
    <Surface gap="control" padding="section">
      <Text textRole="title">Button variants</Text>
      <Surface
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        flexWrap="wrap"
        gap="control"
      >
        <Action onPress={() => undefined}>filled</Action>
        <Action onPress={() => undefined} tone="suggest">
          suggest
        </Action>
        <Action onPress={() => undefined} variant="outline">
          outline
        </Action>
        <Action onPress={() => undefined} tone="danger" variant="outline">
          danger outline
        </Action>
        <Action onPress={() => undefined} variant="ghost">
          ghost
        </Action>
      </Surface>
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Icon only</Text>
      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap="control">
        <Action icon="success" layout="icon" onPress={() => undefined}>
          Save
        </Action>
        <Action icon="close" layout="icon" onPress={() => undefined} variant="ghost">
          Cancel
        </Action>
        <Action icon="delete" layout="icon" onPress={() => undefined} variant="outline">
          Remove
        </Action>
        <Action disabled icon="pageOlder" layout="icon" onPress={() => undefined} variant="outline">
          Back an hour
        </Action>
      </Surface>
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Icon after the label (a pager)</Text>
      <Surface
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap="control"
        justifyContent="space-between"
      >
        <Action icon="pageNewer" onPress={() => undefined} variant="ghost">
          Newer
        </Action>
        <Action icon="pageOlder" iconPlacement="end" onPress={() => undefined} variant="ghost">
          Older
        </Action>
      </Surface>
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Text in a field row</Text>
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap="control"
      >
        <Badge shape="square" tone="override">
          Channel override
        </Badge>
        <Action layout="text" onPress={() => undefined} variant="ghost">
          Reset to default
        </Action>
      </Surface>
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap="control"
      >
        <Badge shape="square" tone="neutral">
          Default
        </Badge>
        <Action disabled layout="text" onPress={() => undefined} variant="ghost">
          Reset to default
        </Action>
      </Surface>
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Composed as a link (web `render` escape hatch)</Text>
      <Action render={<a href="#action-variants-story" />} variant="outline">
        Go to channels
      </Action>
    </Surface>
  </Screen>
);

const meta = {
  title: "Loomarr Foundations/Interaction (Button variants)",
  component: ActionVariantsWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ActionVariantsWorkshop>;

type Story = StoryObj<typeof meta>;
const Default: Story = {};

export default meta;
export { Default };
