import { getAddFillerSourceMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { LocalSourceForm } from "./local-source-form";

const renderForm = () =>
  render(
    <QueryClientProvider
      client={new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })}
    >
      <LocalSourceForm />
    </QueryClientProvider>,
  );

describe("LocalSourceForm", () => {
  it("starts on a media server library and switches the field copy with the kind", async () => {
    renderForm();

    expect(screen.getByRole("textbox", { name: "Library name" })).toHaveAttribute(
      "placeholder",
      expect.stringContaining("Commercials"),
    );

    await userEvent.click(screen.getByRole("combobox", { name: "Kind of source to add" }));
    await userEvent.click(await screen.findByRole("option", { name: "Watched folder" }));

    expect(screen.getByRole("textbox", { name: "Folder path" })).toHaveAttribute(
      "placeholder",
      expect.stringContaining("a full path"),
    );
  });

  it("sends the trimmed address with the chosen kind and clears the field", async () => {
    const adds: unknown[] = [];
    server.use(
      getAddFillerSourceMockHandler(async ({ request }) => {
        adds.push(await request.json());
        return { id: "src-new", label: "New source", uri: "/mnt/extra-ads", enabled: true };
      }),
    );
    renderForm();
    const add = screen.getByRole("button", { name: "+ Add source" });
    expect(add).toBeDisabled();

    await userEvent.type(screen.getByRole("textbox", { name: "Library name" }), "  Commercials  ");
    await userEvent.click(add);

    await waitFor(() => expect(adds).toEqual([{ kind: "library", uri: "Commercials" }]));
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Library name" })).toHaveValue(""));
  });
});
