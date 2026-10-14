import { LoomarrProvider } from "@loomarr/design-system";
import type { Preview } from "@storybook/react-native";
import type { PropsWithChildren } from "react";
import { SafeAreaProvider, useSafeAreaInsets } from "react-native-safe-area-context";

const NativeStoryFrame = ({ children, theme }: PropsWithChildren<{ theme: "dark" | "light" }>) => {
  const insets = useSafeAreaInsets();
  return (
    <LoomarrProvider insets={insets} theme={theme}>
      {children}
    </LoomarrProvider>
  );
};

const preview: Preview = {
  initialGlobals: { theme: "dark" },
  decorators: [
    (Story, context) => (
      <SafeAreaProvider>
        <NativeStoryFrame theme={context.globals.theme === "light" ? "light" : "dark"}>
          <Story />
        </NativeStoryFrame>
      </SafeAreaProvider>
    ),
  ],
};

export default preview;
