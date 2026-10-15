type DeviceSheetAccount = { name: string; role: "admin" | "member" };

type DeviceSheetProps = {
  /** Who the device is signed in as; left out of the sheet until the server has said. */
  account?: DeviceSheetAccount;
  /** The app's version and build, e.g. "1.4.0 (212)". */
  appVersion?: string;
  /** Asks the person to confirm. Defaults to the platform's native action sheet. */
  confirmDisconnect?: (serverAddress: string, onConfirm: () => void) => void;
  /** Revokes this device's credential. A throw keeps the sheet open with a retry. */
  onDisconnect: () => Promise<void> | void;
  onDismiss: () => void;
  /** The paired server's URL; the sheet shows only its host and port. */
  serverUrl: string;
  serverVersion?: string;
};

export type { DeviceSheetAccount, DeviceSheetProps };
