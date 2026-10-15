import { ActionSheetIOS, Alert, Platform } from "react-native";

// Disconnecting revokes the device's credential, so it asks first, in the platform's own words: iOS's
// action sheet with a destructive button above Cancel, Android's alert dialog.
const confirmDisconnect = (serverAddress: string, onConfirm: () => void) => {
  const message = `Loomarr will revoke this device's access to ${serverAddress}. You can pair it again later.`;
  if (Platform.OS === "ios") {
    ActionSheetIOS.showActionSheetWithOptions(
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
  Alert.alert("Disconnect this device?", message, [
    { style: "cancel", text: "Cancel" },
    { onPress: onConfirm, style: "destructive", text: "Disconnect" },
  ]);
};

export { confirmDisconnect };
