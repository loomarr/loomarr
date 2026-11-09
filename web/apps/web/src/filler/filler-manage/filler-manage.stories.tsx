import type { FillerWatchOutputBody, MeBody } from "@loomarr/api";
import type { FillerDefectReportDTO } from "@loomarr/api/models/fillerDefectReportDTO";
import type { ListTaxonomyOutputBody } from "@loomarr/api/models/listTaxonomyOutputBody";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { defectReport } from "@/test/fixtures/filler";
import { withRouter } from "@/test/story-utils";
import { FillerManage } from "./filler-manage";

const admin: MeBody = {
  id: "admin-1",
  name: "Admin",
  role: "admin",
  disabled: false,
  local: true,
  offlineLogin: false,
  quota: 0,
  autoApprove: false,
};

// The Manage hub's reads, typed so a shape change breaks the story build rather than the
// render (an untyped fallback would hand the hub a settings body, #281).
const watch: FillerWatchOutputBody = {
  clips: 1284,
  health: "healthy",
  preparing: 2,
  needsYou: 0,
  sourcesOn: 2,
  sourcesReady: 2,
  sourcesTotal: 3,
};
const hubTaxonomy: ListTaxonomyOutputBody = {
  taxa: [],
  totalClips: 1284,
  taggedClips: 1261,
  unclassifiedClips: 23,
  axisCoverage: [],
};
// The Quality row's read. Without it the hub got the settings fallback and threw on `verdicts`.
const quality: FillerDefectReportDTO = defectReport();

const withManage: Decorator = (Story) => {
  window.fetch = ((input: RequestInfo | URL) => {
    const url = String(input);
    const body = url.includes("/auth/me")
      ? admin
      : url.includes("/filler/watch")
        ? watch
        : url.includes("/taxonomy")
          ? hubTaxonomy
          : url.includes("/filler/defects")
            ? quality
            : { settings: [], features: { filler: true } };
    return Promise.resolve(
      new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } }),
    );
  }) as typeof fetch;
  return (
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <Story />
    </QueryClientProvider>
  );
};

const fillerFrame: Decorator = (Story) => (
  <div style={{ width: "100%", maxWidth: 960 }}>
    <Story />
  </div>
);

const meta = {
  title: "Filler/Manage",
  component: FillerManage,
  tags: ["route"],
  args: { onEditTags: () => {} },
  decorators: [fillerFrame, withManage, withRouter("/filler/manage")],
} satisfies Meta<typeof FillerManage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Hub: Story = {};
