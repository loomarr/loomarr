import { cleanup, renderUi, screen } from "@loomarr/test-support";
import { afterEach, describe, expect, it, vi } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { ChannelDetailProvider, type ChannelDetailValue, useChannelDetail } from "./channel-detail-context";

const Probe = () => {
  const { channel: ch, isAdmin } = useChannelDetail();
  return <p>{`${ch.name}, ${isAdmin ? "admin" : "member"}`}</p>;
};

const value: ChannelDetailValue = {
  id: "ch-1",
  channel: channel(),
  isAdmin: false,
  air: { dot: "live", label: "On air", detail: "" },
  ready: 0,
  total: 0,
  invalidate: () => {},
  saving: false,
  deleting: false,
  save: () => {},
  savePolicy: () => {},
  saveLogo: async () => {},
  onDelete: () => {},
};

afterEach(cleanup);

describe("channel detail context", () => {
  it("hands a section the layout's channel and viewer", () => {
    renderUi(
      <ChannelDetailProvider value={value}>
        <Probe />
      </ChannelDetailProvider>,
    );
    expect(screen.getByText("Late Night Noir, member")).toBeInTheDocument();
  });

  it("refuses a section mounted outside the layout", () => {
    const quiet = vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => renderUi(<Probe />)).toThrow(/inside the channel-detail layout/);
    quiet.mockRestore();
  });
});
