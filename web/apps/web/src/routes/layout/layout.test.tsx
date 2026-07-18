import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { Layout } from "./layout";

describe("Layout", () => {
  it("renders the shell around the routed outlet (SSE stream mounts harmlessly)", () => {
    const qc = new QueryClient();
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={["/channels"]}>
          <Routes>
            <Route element={<Layout />}>
              <Route path="channels" element={<div>outlet content</div>} />
            </Route>
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
    expect(screen.getByRole("navigation", { name: "Primary" })).toBeInTheDocument();
    expect(screen.getByText("outlet content")).toBeInTheDocument();
  });
});
