import * as ReactNative from "react-native";

// react-native-web has no ActionSheetIOS, and a named import of it fails the web bundle; read it off the
// namespace instead, and only on iOS.
const actionSheet = () =>
  (ReactNative as unknown as { ActionSheetIOS: typeof ReactNative.ActionSheetIOS }).ActionSheetIOS;

// Disconnecting revokes the device's credential, so it asks first, in the platform's own words: iOS's
// action sheet with a destructive button above Cancel, Android's alert dialog.
const confirmDisconnect = (serverAddress: string, onConfirm: () => void) => {
  const message = `Loomarr will revoke this device's access to ${serverAddress}. You can pair it again later.`;
  if (ReactNative.Platform.OS === "ios") {
    actionSheet().showActionSheetWithOptions(
      {
        cancelButtonIndex: 1,
        destructiveButtonIndex: 0,
        message,
        options: ["Disconnect", "Cancel"],
        title: "Disconnect this device?",
      },
      (index) => {
        if (index === 0) onConfirm();
      },
    );
    return;
  }
  ReactNative.Alert.alert("Disconnect this device?", message, [
    { style: "cancel", text: "Cancel" },
    { onPress: onConfirm, style: "destructive", text: "Disconnect" },
  ]);
};

export { confirmDisconnect };
