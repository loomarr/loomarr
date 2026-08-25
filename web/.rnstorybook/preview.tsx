import { LoomarrProvider } from "@loomarr/design-system";
import type { Preview } from "@storybook/react-native";

const preview: Preview = {
  initialGlobals: { theme: "dark" },
  decorators: [
    (Story, context) => (
      <LoomarrProvider theme={context.globals.theme === "light" ? "light" : "dark"}>
        <Story />
      </LoomarrProvider>
    ),
  ],
};

export default preview;
