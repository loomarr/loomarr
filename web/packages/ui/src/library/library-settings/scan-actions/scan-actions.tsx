import { Action } from "@loomarr/design-system";
import { useState } from "react";

import { ModalOverlay } from "../../../overlay";
import { Line } from "../../library-poster";
import type { LibrarySettingsProps } from "../library-settings.type";

// Scan now and Refresh everything (Emby and Jellyfin's Scan library and Replace all metadata).
// Scan now starts one pass at once, with no question; Refresh everything is Update details for
// every title, which is slow on a big library, so it asks first.
const ScanActions = ({
  scan,
  onRescan,
  onRefreshEverything,
}: Pick<LibrarySettingsProps, "scan" | "onRescan" | "onRefreshEverything">) => {
  const [asking, setAsking] = useState(false);
  // Looking up details runs on its own clock and never holds up a scan.
  const scanning = scan.step === "walking" || scan.step === "reading";
  return (
    <Line gap="control">
      <ModalOverlay
        actions={[
          { label: "Not now", onPress: () => setAsking(false), preferredFocus: true, tone: "secondary" },
          {
            label: "Refresh everything",
            onPress: () => {
              setAsking(false);
              onRefreshEverything();
            },
            tone: "primary",
          },
        ]}
        description="Loomarr reads every title's details again, from your files and your apps, and looks each one up again. Everything keeps playing. A big library takes a while."
        onDismiss={() => setAsking(false)}
        title="Refresh every title?"
        visible={asking}
      />
      <Action disabled={!scan.enabled || scanning} onPress={onRescan} variant="outline">
        {scanning ? "Scanning…" : "Scan now"}
      </Action>
      <Action disabled={!scan.enabled} onPress={() => setAsking(true)} variant="ghost">
        Refresh everything
      </Action>
    </Line>
  );
};

export { ScanActions };
