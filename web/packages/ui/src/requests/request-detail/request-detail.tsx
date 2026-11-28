import { requestFixLabel } from "@loomarr/core/requests";
import { Action, ScrollFrame, Surface, Text } from "@loomarr/design-system";

import { StatePanel } from "../../state-panel";
import { calendarDay, requestedOn } from "../request-card";
import { RequestsBack } from "../requests-back";
import type { RequestDetailProps } from "./request-detail.type";
import { RequestDetailBody } from "./request-detail-body";

// One request end to end on a touch screen: the way back and the scroll around RequestDetailBody.
const RequestDetail = ({ entry, onBack, onFix, onOpenChannel, timeZone, titles }: RequestDetailProps) => {
  const back = <RequestsBack onPress={onBack} />;
  if (!entry)
    return (
      <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap="control">
        {back}
        <StatePanel
          action={{ label: "Back to Requests", onPress: onBack }}
          density="touch"
          description="It may have been removed. Your other requests are still in the list."
          kind="empty"
          title="That request isn't here"
        />
      </Surface>
    );

  const { journey } = entry;
  const fix = requestFixLabel(journey);

  return (
    <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap="control">
      {back}
      <ScrollFrame density="touch">
        <Text density="touch" textRole="display">
          {journey.intent.description}
        </Text>
        <Text density="touch" textRole="caption" tone="secondary">
          {requestedOn(journey.createdAt, Date.now(), timeZone)}
        </Text>
        <RequestDetailBody
          actions={
            <>
              {journey.channel ? (
                <Action
                  accessibilityRole="button"
                  density="touch"
                  onPress={() => onOpenChannel(journey.channel?.id ?? "")}
                  tone="primary"
                >
                  Open channel
                </Action>
              ) : null}
              {fix ? (
                <Action
                  accessibilityRole="button"
                  density="touch"
                  onPress={() => onFix(entry)}
                  tone="secondary"
                >
                  {fix}
                </Action>
              ) : null}
            </>
          }
          density="touch"
          entry={entry}
          titles={titles}
          whenText={(iso) => `on ${calendarDay(iso, timeZone)}`}
        />
      </ScrollFrame>
    </Surface>
  );
};

export { RequestDetail };
