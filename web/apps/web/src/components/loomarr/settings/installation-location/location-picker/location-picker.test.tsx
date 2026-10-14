import { getLocationsSearchMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";
import { LocationPicker } from "./location-picker";

it("chooses one server-resolved location from the typeahead", async () => {
  const onChange = vi.fn();
  server.use(
    getLocationsSearchMockHandler({
      locations: [
        {
          id: "5129061",
          label: "North Greenbush, United States",
          country: "US",
          market: "North Greenbush",
          region: "New York",
        },
      ],
    }),
  );

  render(<LocationPicker value={{ country: "" }} onChange={onChange} />, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={new QueryClient()}>{children}</QueryClientProvider>
    ),
  });
  await userEvent.type(screen.getByRole("combobox", { name: "Location" }), "North Greenbush");
  await userEvent.click(await screen.findByRole("option", { name: "North Greenbush, United States" }));

  expect(onChange).toHaveBeenCalledWith(
    expect.objectContaining({ country: "US", market: "North Greenbush" }),
  );
});
