import { LiteUI } from "@storybook/react-native-ui-lite";
import { AppRegistry, Platform } from "react-native";

import { view } from "./storybook.requires";
import { TVStorybookUI } from "./tv-ui";

const usesTVNavigator = Platform.isTV || process.env.EXPO_PUBLIC_LOOMARR_STORYBOOK_DENSITY === "tv";

const NativeStorybook = view.getStorybookUI({
  CustomUIComponent: usesTVNavigator ? TVStorybookUI : LiteUI,
  shouldPersistSelection: false,
});

AppRegistry.registerComponent("main", () => NativeStorybook);
