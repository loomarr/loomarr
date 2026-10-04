import { LiteUI } from "@storybook/react-native-ui-lite";
import { AppRegistry, LogBox, Platform } from "react-native";

import { view } from "./storybook.requires";
import { TVStorybookUI } from "./tv-ui";

const usesTVNavigator = Platform.isTV || process.env.EXPO_PUBLIC_LOOMARR_STORYBOOK_DENSITY === "tv";

// Dev only: open straight onto one story (an id such as `loomarr-components-requests--admin-needs-you`),
// so a reviewer can screenshot each state without tapping through the sidebar. Production bundles have
// `__DEV__` false and ignore it; unset, Storybook opens as usual.
const requestedStory = __DEV__ ? process.env.EXPO_PUBLIC_STORYBOOK_INITIAL_STORY : undefined;
const initialStory = requestedStory?.includes("--") ? (requestedStory as `${string}--${string}`) : undefined;

// A pinned story is a review frame: the dev warnings toast would sit over the tab bar's labels.
if (initialStory) LogBox.ignoreAllLogs();

const NativeStorybook = view.getStorybookUI({
  CustomUIComponent: usesTVNavigator ? TVStorybookUI : LiteUI,
  ...(initialStory ? { initialSelection: initialStory } : {}),
  shouldPersistSelection: false,
});

AppRegistry.registerComponent("main", () => NativeStorybook);
