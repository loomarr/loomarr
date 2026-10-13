import { Action, type Density } from "@loomarr/design-system";
import { useState } from "react";

import { ModalOverlay } from "../overlay";

interface AutoTuneSettingActionProps {
  /** The durations offered, in milliseconds (the TV app's per-device choices). */
  choicesMs: readonly number[];
  density: Density;
  onChange: (durationMs: number) => void;
  valueMs: number;
}

// "1.2 s", "2 s". A no-break space keeps the value whole when the row's label wraps.
const formatSeconds = (ms: number) => `${(ms / 1000).toFixed(1).replace(/\.0$/, "")} s`;

/**
 * The TV Surf rail's "Channel-number wait" row (#1659 decision N4, WCAG 2.2.1; approved mock
 * project/prototypes/tv-auto-tune-setting.html). It copies DeviceDisconnectAction's pattern: OK
 * opens a ModalOverlay of choices walked with ◀▶, focus starting on the current value. OK on a
 * choice saves it and closes; BACK closes without a change.
 */
const AutoTuneSettingAction = ({ choicesMs, density, onChange, valueMs }: AutoTuneSettingActionProps) => {
  const [choosing, setChoosing] = useState(false);
  return (
    <>
      <Action density={density} onPress={() => setChoosing(true)} tone="secondary">
        {`Channel-number wait · ${formatSeconds(valueMs)}`}
      </Action>
      <ModalOverlay
        actions={choicesMs.map((ms) => ({
          label: formatSeconds(ms),
          onPress: () => {
            onChange(ms);
            setChoosing(false);
          },
          preferredFocus: ms === valueMs,
          selected: ms === valueMs,
          tone: "secondary" as const,
        }))}
        density={density}
        description="After you type a channel number, Loomarr waits this long before it tunes. OK tunes at once."
        onDismiss={() => setChoosing(false)}
        title="Channel-number wait"
        visible={choosing}
      />
    </>
  );
};

export type { AutoTuneSettingActionProps };
export { AutoTuneSettingAction };
