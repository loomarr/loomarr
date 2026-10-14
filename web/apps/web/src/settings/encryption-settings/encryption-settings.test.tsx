import {
  getSystemEncryptionRotateDataKeyMockHandler,
  getSystemEncryptionStatusMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { EncryptionSettings } from "./encryption-settings";

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

describe("EncryptionSettings", () => {
  it("shows the non-secret fingerprint and rotates the data key", async () => {
    let rotations = 0;
    server.use(
      getSystemEncryptionStatusMockHandler({
        enabled: true,
        installationKeyFingerprint: "sha256:0123456789abcdef",
        dataKeyCount: 1,
      }),
      getSystemEncryptionRotateDataKeyMockHandler(() => {
        rotations += 1;
      }),
    );
    render(<EncryptionSettings />, { wrapper });

    expect(await screen.findByText("sha256:0123456789abcdef")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Rotate data key" }));

    expect(await screen.findByText(/Stored settings were re-encrypted/)).toBeInTheDocument();
    expect(rotations).toBe(1);
  });
});
