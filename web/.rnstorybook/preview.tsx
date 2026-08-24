import { LoomarrProvider } from "@loomarr/design-system";
import type { Preview } from "@storybook/react-native";

const preview: Preview = {
  decorators: [
    (Story) => (
      <LoomarrProvider>
        <Story />
      </LoomarrProvider>
    ),
  ],
};

export default preview;
