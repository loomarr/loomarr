import { Action, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// A form's submit button and the compact icon action (#2147), in their own file so the existing
// Action stories keep their visual baselines.
const ActionCompactSubmitWorkshop = () => (
  <Screen gap="section">
    <Surface gap="control" padding="section">
      <Text textRole="title">Submit</Text>
      <form
        onSubmit={(event) => event.preventDefault()}
        style={{ alignItems: "center", display: "flex", gap: 12 }}
      >
        <Action submit>Save</Action>
        <Action disabled submit>
          Save
        </Action>
        <Action submit variant="suggest">
          Suggest
        </Action>
      </form>
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Compact icon</Text>
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap="control"
      >
        <Action compact icon="close" layout="icon" onPress={() => undefined} variant="ghost">
          Remove Comedy
        </Action>
        <Action compact icon="dragHandle" layout="icon" onPress={() => undefined} variant="ghost">
          Reorder Comedy
        </Action>
        <Action compact icon="info" layout="icon" onPress={() => undefined} variant="outline">
          About this setting
        </Action>
        <Action compact disabled icon="close" layout="icon" onPress={() => undefined} variant="ghost">
          Remove
        </Action>
      </Surface>
    </Surface>
  </Screen>
);

const meta = {
  title: "Loomarr Components/Action submit and compact",
  component: ActionCompactSubmitWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ActionCompactSubmitWorkshop>;

type Story = StoryObj<typeof meta>;
const Workshop: Story = {};

export default meta;
export { Workshop };
