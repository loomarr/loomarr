import { SelectControl, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { View } from "react-native";

import type { ChannelAudioSectionProps } from "./channel-audio-section.type";

// CHANNEL AUDIO — the channel page's default audio language (#2088), the card under On-screen logo
// on Channel info. Presentational: the app owns the tracks, the saved value and the autosave. It is
// the channel's DEFAULT; the viewer's own choice between renditions comes with alternate audio.

const LABEL = "Default audio language";

const ChannelAudioSection = ({ options, onValueChange, value }: ChannelAudioSectionProps) => {
  const [open, setOpen] = useState(false);
  return (
    <Surface aria-label="Audio" backgroundColor="surfaceRaised" gap={20} padding={20} role="region">
      {/* The same header roles as the On-screen logo section above it. */}
      <View style={{ gap: 4 }}>
        <Text textRole="title">Audio</Text>
        <Text textRole="hint" tone="muted">
          The language everyone watching this channel hears, when the programme carries it.
        </Text>
      </View>
      <View style={{ gap: 8, maxWidth: 320 }}>
        <Text textRole="label">{LABEL}</Text>
        <SelectControl
          appearance="field"
          label={LABEL}
          onOpenChange={setOpen}
          onValueChange={onValueChange}
          open={open}
          options={options}
          value={value}
        />
      </View>
    </Surface>
  );
};

export { ChannelAudioSection };
