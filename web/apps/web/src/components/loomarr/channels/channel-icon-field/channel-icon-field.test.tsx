import { getChannelIconSuggestionsMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { render } from "@/test/render";
import { ChannelIconField } from "./channel-icon-field";

const renderField = (onSetLogo: (logo: string) => Promise<void>, isAdmin = true) => {
  server.use(getChannelIconSuggestionsMockHandler({ suggestions: [] }));
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <ChannelIconField channelId="ch-1" isAdmin={isAdmin} onSetLogo={onSetLogo} />
    </QueryClientProvider>,
  );
};

describe("ChannelIconField", () => {
  it("sets a pasted URL when Enter is pressed in the box", async () => {
    const onSetLogo = vi.fn().mockResolvedValue(undefined);
    renderField(onSetLogo);

    await userEvent.click(screen.getByRole("button", { name: "Change…" }));
    await userEvent.type(
      screen.getByLabelText("Paste an image URL"),
      " https://example.com/icon.png {Enter}",
    );

    await waitFor(() => expect(onSetLogo).toHaveBeenCalledWith("https://example.com/icon.png"));
    expect(onSetLogo).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.getByLabelText("Paste an image URL")).toHaveValue(""));
  });

  it("keeps the URL and shows why when it cannot be set", async () => {
    const onSetLogo = vi.fn().mockRejectedValue(new Error("That URL is not an image."));
    renderField(onSetLogo);

    await userEvent.click(screen.getByRole("button", { name: "Change…" }));
    await userEvent.type(screen.getByLabelText("Paste an image URL"), "https://example.com/icon.png{Enter}");

    expect(await screen.findByText("That URL is not an image.")).toBeInTheDocument();
    expect(screen.getByLabelText("Paste an image URL")).toHaveValue("https://example.com/icon.png");
  });

  it("offers a viewer the icon but no way to change it", () => {
    renderField(vi.fn(), false);
    expect(screen.queryByRole("button", { name: "Change…" })).not.toBeInTheDocument();
  });
});
