import { Action, RadioGroup, StatusDot, Surface, Text, useWindowWidth } from "@loomarr/design-system";
import { useState } from "react";

import { LibraryPoster, Line, Stack } from "../../library-poster";
import type { LibraryOrderNeed } from "../library-needs.type";
import { LockIn } from "../lock-in";
import { NotSaved } from "../not-saved";
import { OrderFiles } from "../order-files";
import { FILES, mismatchLine, type OrderOption, onlyCatalog, orderOptions } from "../order-options";
import { OrderRadio } from "../order-radio";
import { plural } from "../plural";

const OrderCard = ({
  need,
  primary,
  done,
  failed,
  onChoose,
  onDone,
  onOpenTitle,
}: {
  need: LibraryOrderNeed;
  primary: boolean;
  /** The answer given, as the done line says it; absent while it is asked. */
  done?: { line: string; fromTmdb: boolean };
  /** The last answer was not saved. */
  failed?: boolean;
  onChoose: (option: OrderOption) => void;
  onDone: () => void;
  onOpenTitle: (id: string) => void;
}) => {
  const options = orderOptions(need);
  const [picked, setPicked] = useState(() => (options.find((o) => o.fits) ?? options[0])?.key ?? FILES);
  const wide = useWindowWidth() >= 640;
  const waiting = need.files.length;
  const chosen = options.find((o) => o.key === picked);
  const tone = primary ? { tone: "primary" as const } : { variant: "outline" as const };
  const files = options.find((o) => o.key === FILES);
  const card = (
    <Surface gap="control" padding="section">
      <Line alignItems="flex-start" columnGap="section" flexWrap={wide ? "nowrap" : "wrap"} rowGap="control">
        <Stack flexShrink={0} width={wide ? 144 : 112}>
          <LibraryPoster art={need.poster} big name={need.name} />
        </Stack>
        <Stack flexBasis={240} flexGrow={1} flexShrink={1} gap="control" minWidth={0}>
          {done ? (
            <>
              <Line paddingTop="section">
                <StatusDot label="Done" tone="ok" />
                <Text textRole="title">{`Done — ${need.name} ${done.line}.`}</Text>
              </Line>
              {done.fromTmdb ? (
                <Text textRole="compact" tone="muted">
                  {`${plural(waiting, "episode")} now ${waiting === 1 ? "has its" : "have their"} names and details from TMDB.`}
                </Text>
              ) : null}
            </>
          ) : onlyCatalog(need) ? (
            <>
              <Text textRole="title">{`Some ${need.name} episodes don't line up with TMDB`}</Text>
              <Text textRole="body" tone="secondary">
                {mismatchLine(need)}
              </Text>
              <Line>
                <Action onPress={() => files && onChoose(files)} {...tone}>
                  That's fine
                </Action>
                <Action onPress={() => onOpenTitle(need.id)} variant="outline">
                  See the show
                </Action>
              </Line>
              {failed ? <NotSaved /> : null}
              <OrderFiles files={need.files} />
            </>
          ) : (
            <>
              <Text textRole="title">{`Which order are your ${need.name} episodes in?`}</Text>
              <Text textRole="body" tone="secondary">
                {`TMDB lists this show in more than one order, and your files don't say which they follow. Until you choose, ${plural(waiting, "episode")} use the name and number in ${waiting === 1 ? "its" : "their"} file.`}
              </Text>
              <RadioGroup
                borderColor="borderDecorative"
                borderRadius="control"
                borderWidth={1}
                label="Episode order"
                onValueChange={setPicked}
                overflow="hidden"
                values={options.map((o) => o.key)}
                value={picked}
              >
                {options.map((o, i) => (
                  <Stack
                    borderColor="borderDecorative"
                    borderRadius={0}
                    borderTopWidth={i === 0 ? 0 : 1}
                    key={o.key}
                  >
                    <OrderRadio option={o} />
                  </Stack>
                ))}
              </RadioGroup>
              <Line>
                <Action onPress={() => chosen && onChoose(chosen)} {...tone}>
                  {picked === FILES ? "Keep the file names" : "Use this order"}
                </Action>
              </Line>
              {failed ? <NotSaved /> : null}
              <OrderFiles files={need.files} />
            </>
          )}
        </Stack>
      </Line>
    </Surface>
  );
  return done ? <LockIn onDone={onDone}>{card}</LockIn> : card;
};

export { OrderCard };
