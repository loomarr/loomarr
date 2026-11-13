import { Icon, icons, StatusDot, Text } from "@loomarr/design-system";

import { Line, Stack } from "../../library-poster";
import { type detailsWaiting, folderName, joinNames } from "../../library-words";
import type { LibraryFolder, LibraryHook } from "../library-settings.type";

const HOOK_NAMES: Record<LibraryHook["app"], string> = { sonarr: "Sonarr", radarr: "Radarr" };

// What keeps the library current: the folder watch, and each connected arr's webhook.
const KeepingUp = ({
  offline,
  hooks,
  waiting,
}: {
  offline: readonly LibraryFolder[];
  hooks: readonly LibraryHook[];
  /** Titles stuck waiting for their details, and why; undefined when none are. */
  waiting?: ReturnType<typeof detailsWaiting>;
}) => {
  const signals = [
    {
      key: "watch",
      label: "Loomarr notices new and deleted files on its own",
      waitingFor: offline.length > 0 ? joinNames(offline.map((f) => folderName(f.path))) : "",
    },
    ...hooks
      .filter((hook) => hook.state !== "not_configured")
      .map((hook) => ({
        key: hook.app,
        label: `${HOOK_NAMES[hook.app]} tells Loomarr about new downloads`,
        waitingFor:
          hook.state === "registered"
            ? ""
            : hook.state === "no_address"
              ? "Loomarr's address"
              : HOOK_NAMES[hook.app],
      })),
  ];
  return (
    <Stack gap="control">
      <Text textRole="label" tone="secondary">
        Keeping up to date
      </Text>
      <Stack gap="inline">
        {signals.map((signal) => (
          <Line alignItems="flex-start" flexWrap="nowrap" gap="inline" key={signal.key}>
            {signal.waitingFor ? (
              <StatusDot label="Waiting" size="compact" tone="warn" />
            ) : (
              <Icon decorative glyph={icons.success} size="compact" tone="success" />
            )}
            <Text textRole="body">
              {signal.waitingFor ? `${signal.label} — waiting for ${signal.waitingFor}` : signal.label}
            </Text>
          </Line>
        ))}
        {waiting ? (
          <Line alignItems="flex-start" flexWrap="nowrap" gap="inline">
            <StatusDot label="Waiting" size="compact" tone="warn" />
            <Stack flexShrink={1} gap="inline">
              <Text textRole="body">{waiting.title}</Text>
              <Text textRole="body" tone="secondary">
                {waiting.reason}
              </Text>
            </Stack>
          </Line>
        ) : null}
      </Stack>
    </Stack>
  );
};

export { KeepingUp };
