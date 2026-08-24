import { type GuideSelection, layoutGuide } from "@loomarr/core";
import { Screen, Surface, Text } from "@loomarr/design-system";
import { guideChannels, guideFrom, guideNow, guideTo } from "@loomarr/fixtures";
import { GuideSurface } from "@loomarr/ui";
import { activateTvGuideFocus, moveTvGuideFocus, type TvGuideNavigationState } from "@loomarr/ui-tv";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const layout = layoutGuide(
  { channels: guideChannels, fromMs: guideFrom, timezone: "America/New_York", toMs: guideTo },
  guideNow,
);
const filters = [{ value: "all" }, { disabled: true, value: "favourites" }, { value: "recent" }] as const;
const initialSelection: GuideSelection = {
  anchorMs: guideFrom + 17 * 60_000,
  channelId: "ch-action",
  scheduleBlockId: "block_action_heat",
};

const TvGuideWorkshop = () => {
  const [navigation, setNavigation] = useState<TvGuideNavigationState>({
    activeFilter: "all",
    focus: { region: "grid", selection: initialSelection },
    gridSelection: initialSelection,
  });
  const [announcement, setAnnouncement] = useState("Guide focused");

  return (
    <div
      aria-label="TV Guide remote controller"
      onKeyDown={(event) => {
        const direction = {
          ArrowDown: "down",
          ArrowLeft: "left",
          ArrowRight: "right",
          ArrowUp: "up",
        }[event.key] as "down" | "left" | "right" | "up" | undefined;
        if (direction) {
          event.preventDefault();
          const next = moveTvGuideFocus(layout, navigation, direction, filters).state;
          setNavigation(next);
          setAnnouncement(
            next.focus.region === "filters"
              ? `${next.focus.filter} filter focused`
              : `${next.focus.selection.channelId} focused`,
          );
        }
        if (event.key === "Enter") {
          const activation = activateTvGuideFocus(navigation);
          if (activation.kind === "filter") {
            setNavigation((current) => ({ ...current, activeFilter: activation.filter }));
            setAnnouncement(`${activation.filter} filter applied`);
          } else {
            setAnnouncement(`${activation.selection.channelId} tuned`);
          }
        }
      }}
      role="application"
      style={{ minHeight: "100vh", outline: "none" }}
      // Composite remote surface: one focus owner translates D-pad keys for its child controls.
      // biome-ignore lint/a11y/noNoninteractiveTabindex: role=application is deliberately focusable
      tabIndex={0}
    >
      <Screen density="tv" gap="$control" justifyContent="center">
        <Surface
          alignItems="center"
          backgroundColor="$transparent"
          borderWidth={0}
          flexDirection="row"
          justifyContent="space-between"
        >
          <Text textRole="metadata" tone="info">
            D-PAD WORKSHOP
          </Text>
          <Text aria-live="polite" textRole="metadata">
            {announcement}
          </Text>
        </Surface>
        <GuideSurface
          density="tv"
          filter={navigation.activeFilter as "all" | "favourites" | "recent"}
          layout={layout}
          onFilterChange={(activeFilter) => setNavigation((current) => ({ ...current, activeFilter }))}
          onSelectionChange={(selection) =>
            setNavigation((current) => ({
              ...current,
              focus: { region: "grid", selection },
              gridSelection: selection,
            }))
          }
          selection={navigation.gridSelection}
        />
      </Screen>
    </div>
  );
};

const meta = {
  title: "Loomarr Components/TV Guide Adapter",
  component: TvGuideWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof TvGuideWorkshop>;

type Story = StoryObj<typeof meta>;
const Remote: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("application", { name: "TV Guide remote controller" }).focus();
  },
};
const LightRemote: Story = { ...Remote, globals: { theme: "light" } };

export default meta;
export { LightRemote, Remote };
