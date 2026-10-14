import type { StorybookConfig } from "@storybook/react-native";

const main: StorybookConfig = {
  stories: ["../native-stories/**/*.stories.?(ts|tsx|js|jsx)"],
};

export default main;
