import { expect, test } from "@playwright/test";
import { installMockBackend } from "./mock-backend";

const response = (body: unknown) => ({
  status: 200,
  contentType: "application/json",
  body: JSON.stringify(body),
});

test("AI setup leads with useful choices and searches the live catalog", async ({ page }) => {
  await installMockBackend(page, { authed: true, role: "admin", checks: { llm: false } });

  await page.route("**/v1/settings", (route) =>
    route.fulfill(
      response({
        features: {},
        settings: [
          {
            key: "llm.provider",
            label: "Lineup AI provider",
            group: "ai",
            kind: "enum",
            enumOptions: [
              { value: "ollama", label: "Ollama" },
              { value: "openai", label: "OpenAI-compatible" },
            ],
            doc: "Where lineup suggestions run.",
            advanced: false,
            secret: false,
            set: true,
            provenance: "db",
            value: "openai",
          },
          {
            key: "llm.url",
            label: "AI service address",
            group: "ai",
            kind: "url",
            doc: "The OpenAI-compatible API base.",
            advanced: false,
            secret: false,
            set: true,
            provenance: "db",
            value: "https://openrouter.ai/api/v1",
          },
          {
            key: "llm.api_key",
            label: "Hosted AI API key",
            group: "ai",
            kind: "secret",
            doc: "The hosted-provider credential.",
            advanced: false,
            secret: true,
            set: true,
            preview: "…e1fe",
            provenance: "db",
            value: "",
          },
          {
            key: "suggest.max_acquisitions",
            label: "Maximum acquisitions per suggestion",
            group: "ai",
            kind: "int",
            doc: "Bounds suggestion work.",
            advanced: false,
            secret: false,
            set: true,
            provenance: "db",
            value: "5",
          },
        ],
      }),
    ),
  );
  await page.route("**/v1/system/llm", (route) =>
    route.fulfill(
      response({
        provider: "openai",
        model: "",
        local: false,
        reachable: true,
        catalog: [],
        hosted: [
          {
            key: "openrouter",
            label: "OpenRouter",
            baseUrl: "https://openrouter.ai/api/v1",
            keyConfigured: true,
            active: true,
            modelsLive: true,
            models: [
              {
                id: "openai/gpt-5.4-mini",
                label: "OpenAI: GPT-5.4 Mini",
                recommended: true,
                tools: true,
                why: "Best balance — strong reasoning without flagship pricing.",
              },
              {
                id: "google/gemini-3-flash-preview",
                label: "Google: Gemini 3 Flash Preview",
                tools: true,
                why: "Best value — fast and capable.",
              },
              {
                id: "openai/gpt-5.6-sol",
                label: "OpenAI: GPT-5.6 Sol",
                tools: true,
                why: "Highest quality — flagship reasoning.",
              },
              { id: "openai/gpt-5.6-luna", label: "OpenAI: GPT-5.6 Luna", tools: true },
            ],
          },
          {
            key: "custom",
            label: "Custom",
            baseUrl: "",
            keyConfigured: false,
            active: false,
            modelsLive: false,
            models: [],
          },
        ],
      }),
    ),
  );

  await page.goto("/settings/ai");

  await expect(page.getByRole("heading", { name: "Choose a lineup model" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Automatic model policy" })).toHaveCount(0);
  await expect(page.getByRole("listitem").filter({ hasText: "OpenAI: GPT-5.4 Mini" })).toBeVisible();
  await expect(page.getByRole("listitem").filter({ hasText: "OpenAI: GPT-5.6 Luna" })).toHaveCount(0);
  await expect(page.getByText("Custom", { exact: true })).toHaveCount(0);

  await page.getByRole("button", { name: "Find another model" }).click();
  const search = page.getByRole("searchbox", { name: "Search models" });
  await search.fill("luna");
  await expect(page.getByRole("listitem").filter({ hasText: "OpenAI: GPT-5.6 Luna" })).toBeVisible();
  await expect(page.getByRole("listitem").filter({ hasText: "OpenAI: GPT-5.4 Mini" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Back to recommendations" })).toBeVisible();
});
