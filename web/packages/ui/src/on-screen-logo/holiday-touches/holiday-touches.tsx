import {
  Action,
  Badge,
  FieldHint,
  RadioGroup,
  RadioItem,
  Surface,
  Text,
  Toggle,
} from "@loomarr/design-system";
import { View } from "react-native";
import { day } from "../choices";
import type { OnScreenLogoSectionProps } from "../on-screen-logo.type";
import { MotifIcon } from "../preview-parts";
import { group, row } from "../styles";
import type { LogoView } from "../use-logo-view";

type HolidayTouchesProps = Pick<
  OnScreenLogoSectionProps,
  "everyHoliday" | "holidays" | "onChange" | "onOpenProgramming" | "policy" | "previewSrc"
> & { view: LogoView };

// The body of the "Holiday touches" row: the switch, one bordered row per holiday with its picture
// chips, and the sentences about when the change happens.
const HolidayTouches = ({
  everyHoliday,
  holidays,
  onChange,
  onOpenProgramming,
  policy: p,
  previewSrc,
  view,
}: HolidayTouchesProps) => {
  if (view.ignoresHolidays) {
    return (
      <View style={{ gap: 6 }}>
        <Text textRole="hint" tone="muted">
          This channel ignores holidays, so its logo stays the same all year. To change that, set Holidays on
          its Programming tab.
        </Text>
        <Action onPress={onOpenProgramming} tone="secondary" variant="ghost">
          Open Programming
        </Action>
      </View>
    );
  }
  return (
    <View style={{ gap: 6 }}>
      <Toggle
        checked={view.holidayOn}
        kind="switch"
        label={view.holidayOn ? "On" : "Off"}
        onCheckedChange={(on) => onChange({ holiday: on })}
      />
      {view.holidayOn ? (
        <>
          {holidays.map((h) => (
            // The mock's .hrow: one bordered row per holiday, its pictures as small chips.
            <Surface
              alignItems="center"
              backgroundColor="transparent"
              borderColor="borderSubtle"
              borderRadius="control"
              borderWidth={1}
              columnGap={14}
              flexDirection="row"
              flexWrap="wrap"
              paddingHorizontal={12}
              paddingVertical={10}
              rowGap={8}
              key={h.id}
            >
              <View style={{ minWidth: 108 }}>
                <Text textRole="label">{h.name}</Text>
              </View>
              <FieldHint>{`${day(h.start)} – ${day(h.end)}`}</FieldHint>
              {h.active ? (
                <Badge shape="square" tone="lock">
                  In effect now
                </Badge>
              ) : null}
              {h.pictures.length > 1 ? (
                <RadioGroup
                  label={`Picture for ${h.name}`}
                  onValueChange={(id) => onChange({ motifs: { ...p.motifs, [h.id]: id } })}
                  value={h.picture}
                  values={h.pictures.map((m) => m.id)}
                  {...group}
                  gap={6}
                >
                  {h.pictures.map((m) => (
                    <RadioItem key={m.id} label={`${m.name} for ${h.name}`} value={m.id}>
                      {({ checked }) => (
                        <Surface
                          alignItems="center"
                          backgroundColor={checked ? "surfaceFocus" : "transparent"}
                          borderColor={checked ? "contentPrimary" : "borderControl"}
                          // The 2px border takes its extra pixel from the padding, so the chip
                          // keeps its size when chosen.
                          borderRadius="control"
                          borderWidth={checked ? 2 : 1}
                          flexDirection="row"
                          gap={6}
                          minHeight={36}
                          paddingLeft={checked ? 5 : 6}
                          paddingRight={checked ? 9 : 10}
                          paddingVertical={checked ? 3 : 4}
                        >
                          <MotifIcon src={previewSrc({ holiday: h.id, motifOnly: true, picture: m.id })} />
                          <Text strong={checked} textRole="cardLabel">
                            {m.name}
                          </Text>
                        </Surface>
                      )}
                    </RadioItem>
                  ))}
                </RadioGroup>
              ) : (
                <View style={{ ...row, gap: 6 }}>
                  <MotifIcon src={previewSrc({ holiday: h.id, motifOnly: true })} />
                  <Text textRole="cardLabel">{h.pictures[0]?.name ?? ""}</Text>
                </View>
              )}
            </Surface>
          ))}
          <FieldHint>
            {`Holidays come from this channel’s Programming tab (${everyHoliday ? "it uses every built-in holiday" : `it uses ${holidays.map((h) => h.name).join(", ")}`}). ${view.holSentence} ${holidays.length > 1 ? "The other holidays follow the dates above; on all other days it shows the everyday logo." : "On every other day it shows the everyday logo."}`}
          </FieldHint>
          <FieldHint>
            The change happens at the next programme that starts after the date. A programme already airing
            keeps its logo until it ends.
          </FieldHint>
        </>
      ) : (
        <FieldHint>Off: the everyday logo airs all year.</FieldHint>
      )}
    </View>
  );
};

export { HolidayTouches };
