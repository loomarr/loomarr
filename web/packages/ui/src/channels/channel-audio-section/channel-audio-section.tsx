import { SelectControl, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { View } from "react-native";

import type { ChannelAudioOption, ChannelAudioSectionProps } from "./channel-audio-section.type";

// CHANNEL AUDIO AND SUBTITLES — the channel page's default audio language and burned-in subtitles
// (#2088), the card under On-screen logo on Channel info. Presentational: the app owns the tracks,
// the saved values and the autosave. Both are the CHANNEL's: the viewer's own audio and subtitle
// choices live in the player.

const AUDIO = "Default audio language";
const SUBTITLES = "Burned-in subtitles";

interface FieldProps {
  hint?: string;
  label: string;
  onValueChange: (value: string) => void;
  options: readonly ChannelAudioOption[];
  value: string;
}

const Field = ({ hint, label, onValueChange, options, value }: FieldProps) => {
  const [open, setOpen] = useState(false);
  return (
    <View style={{ gap: 8, maxWidth: 320 }}>
      <Text textRole="label">{label}</Text>
      <SelectControl
        appearance="field"
        label={label}
        onOpenChange={setOpen}
        onValueChange={onValueChange}
        open={open}
        options={options}
        value={value}
      />
      {hint ? (
        <Text textRole="hint" tone="muted">
          {hint}
        </Text>
      ) : null}
    </View>
  );
};

const ChannelAudioSection = ({ options, onValueChange, subtitles, value }: ChannelAudioSectionProps) => (
  <Surface
    aria-label="Audio and subtitles"
    backgroundColor="surfaceRaised"
    gap={20}
    padding={20}
    role="region"
  >
    {/* The same header roles as the On-screen logo section above it. */}
    <View style={{ gap: 4 }}>
      <Text textRole="title">Audio and subtitles</Text>
      <Text textRole="hint" tone="muted">
        What everyone watching this channel hears and sees, when the programme carries it.
      </Text>
    </View>
    <Field label={AUDIO} onValueChange={onValueChange} options={options} value={value} />
    <Field
      hint="Blu-ray image subtitles are drawn into the picture for everyone."
      label={SUBTITLES}
      onValueChange={subtitles.onValueChange}
      options={subtitles.options}
      value={subtitles.value}
    />
  </Surface>
);

export { ChannelAudioSection };
