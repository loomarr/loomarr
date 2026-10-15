// react-native-web has no ActionSheetIOS, so the web build asks with the browser's own confirm dialog. The
// phone shell is the only host that opens the Device sheet; this exists so the shared package still bundles
// for web. (This package's tsconfig has no DOM lib, hence the narrow cast.)
const confirmDisconnect = (serverAddress: string, onConfirm: () => void) => {
  const { confirm } = globalThis as unknown as { confirm: (message: string) => boolean };
  if (confirm(`Disconnect this device? Loomarr will revoke its access to ${serverAddress}.`)) onConfirm();
};

export { confirmDisconnect };
