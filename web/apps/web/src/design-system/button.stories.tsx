import { Action, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The four button looks (#2777): primary-filled, secondary-outline, text-ghost and icon-ghost. The
// rule they carry: one filled button per screen, the screen's main action. Danger is a tone on the
// outline, never a fill; the AI accent is a tone on the one filled button.
const none = () => undefined;

const Row = ({ title, children }: { title: string; children: React.ReactNode }) => (
  <Surface gap="control" padding="section">
    <Text textRole="title">{title}</Text>
    <Surface
      alignItems="center"
      backgroundColor="transparent"
      borderWidth={0}
      flexDirection="row"
      flexWrap="wrap"
      gap="control"
    >
      {children}
    </Surface>
  </Surface>
);

const FourVariants = () => (
  <Screen gap="section">
    <Row title="Primary-filled: the one main action">
      <Action onPress={none}>Invite person</Action>
      <Action disabled onPress={none}>
        Invite person
      </Action>
    </Row>
    <Row title="Secondary-outline: every other action beside it">
      <Action onPress={none} variant="outline">
        Add local account
      </Action>
      <Action onPress={none} tone="danger" variant="outline">
        Delete channel
      </Action>
    </Row>
    <Row title="Text-ghost: the quietest action">
      <Action onPress={none} variant="ghost">
        Cancel
      </Action>
    </Row>
    <Row title="Icon-ghost: an icon alone, named for assistive tech">
      <Action icon="close" layout="icon" onPress={none} variant="ghost">
        Close
      </Action>
      <Action icon="search" layout="icon" onPress={none} variant="ghost">
        Search
      </Action>
    </Row>
    <Row title="One filled button per screen (People)">
      <Action onPress={none}>Invite person</Action>
      <Action onPress={none} variant="outline">
        Add local account
      </Action>
      <Action onPress={none} variant="outline">
        Import from Emby/Jellyfin
      </Action>
    </Row>
    <Row title="The AI accent, as the one filled button">
      <Action onPress={none} tone="suggest">
        Suggest a lineup
      </Action>
    </Row>
  </Screen>
);

const meta = {
  title: "Primitives/Button",
  component: FourVariants,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof FourVariants>;

type Story = StoryObj<typeof meta>;
const Default: Story = {};
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Default, Light };
