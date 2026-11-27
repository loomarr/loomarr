import { Action } from "@loomarr/design-system";
import { useState } from "react";

import { ModalOverlay } from "../../../overlay";
import { folderName } from "../../library-words";
import type { LibraryFolder } from "../library-settings.type";

// Releasing a folder forgets its files at once, so it asks first and says what goes with them. The
// server also drops a released folder that no app names (at once when added by hand, else at the
// next discovery), so that one has to be added again.
const StopUsing = ({ folder, onConfirm }: { folder: LibraryFolder; onConfirm: () => void }) => {
  const [asking, setAsking] = useState(false);
  const back = folder.foundIn.length === 0 ? "add the folder" : "use the folder";
  return (
    <>
      <ModalOverlay
        actions={[
          // First, so the web dialog puts focus on it when it opens.
          { label: "Cancel", onPress: () => setAsking(false), preferredFocus: true, tone: "secondary" },
          {
            label: "Stop using",
            onPress: () => {
              setAsking(false);
              onConfirm();
            },
            tone: "danger",
          },
        ]}
        description={`Titles found only in this folder leave your Library, and channels can't play them. Your files stay where they are. To bring them back, ${back} again.`}
        onDismiss={() => setAsking(false)}
        title={`Stop using ${folderName(folder.path)}?`}
        visible={asking}
      />
      <Action onPress={() => setAsking(true)} variant="ghost">
        Stop using
      </Action>
    </>
  );
};

export { StopUsing };
