import { mkdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { expect, test } from "@playwright/test";

const packageSHA256 = "a".repeat(64);
const tinyPNG = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
  "base64",
);

test("offline temporal reviewer completes by keyboard, resumes, navigates, controls speed, and exports", async ({
  context,
  page,
}, testInfo) => {
  const root = testInfo.outputPath("temporal-reviewer");
  await mkdir(root, { recursive: true });
  await writeFile(path.join(root, "frame.png"), tinyPNG);
  await writeFile(path.join(root, "review.mp4"), Buffer.from("intentionally inert test media"));

  const reviewPackage = {
    schemaVersion: 1,
    contractVersion: "filler-temporal-human-review-v1",
    questionVersion: "filler-temporal-two-question-v1",
    batchId: "browser-test-batch",
    preparedAt: "2026-09-01T12:00:00Z",
    evidenceManifestSha256: "b".repeat(64),
    selectionSha256: "c".repeat(64),
    seedSha256: "d".repeat(64),
    cases: [
      {
        alias: `review-${"1".repeat(32)}`,
        durationMs: 1_000,
        video: {
          path: "review.mp4",
          sha256: "e".repeat(64),
          bytes: 30,
          durationMs: 1_000,
          width: 320,
          height: 240,
        },
        frames: [
          {
            id: "frame-01",
            path: "frame.png",
            sha256: "f".repeat(64),
            bytes: tinyPNG.length,
            width: 1,
            height: 1,
            atMs: 0,
            ocr: [{ text: "SALE", confidence: 0.9, x: 0, y: 0, width: 1, height: 1 }],
          },
        ],
        transcriptSegments: [{ id: "transcript-01", startMs: 0, endMs: 500, text: "A short spoken line." }],
      },
      {
        alias: `review-${"2".repeat(32)}`,
        durationMs: 1_000,
        video: {
          path: "review.mp4",
          sha256: "e".repeat(64),
          bytes: 30,
          durationMs: 1_000,
          width: 320,
          height: 240,
        },
        frames: [
          {
            id: "frame-01",
            path: "frame.png",
            sha256: "f".repeat(64),
            bytes: tinyPNG.length,
            width: 1,
            height: 1,
            atMs: 0,
          },
        ],
      },
    ],
  };
  const templatePath = path.resolve(
    testInfo.config.rootDir,
    "../../../../../internal/fillerreview/temporal_reviewer_template.go",
  );
  const templateSource = await readFile(templatePath, "utf8");
  const templateStart = templateSource.indexOf("`<!doctype html>");
  if (templateStart < 0) throw new Error("temporal reviewer template start was not found");
  const template = templateSource.slice(templateStart + 1, templateSource.lastIndexOf("`"));
  const rendered = template
    .replace("__LOOMARR_TEMPORAL_REVIEW_PACKAGE_JSON__", JSON.stringify(reviewPackage))
    .replace("__LOOMARR_TEMPORAL_REVIEW_PACKAGE_SHA256__", packageSHA256);
  const viewerPath = path.join(root, "index.html");
  await writeFile(viewerPath, rendered);

  const requests: string[] = [];
  page.on("request", (request) => requests.push(request.url()));
  await context.setOffline(true);
  await page.goto(pathToFileURL(viewerPath).href);
  await expect(page.getByText("Case 1 of 2")).toBeVisible();
  await expect(page.getByText("SALE")).toBeVisible();
  await expect(page.getByText("A short spoken line.")).toBeVisible();

  await page.getByRole("textbox", { name: "Reviewer identity" }).fill("reviewer-browser");
  await page.getByRole("combobox", { name: "Speed" }).selectOption("1.5");
  await page.getByRole("heading", { name: "Unit structure" }).click();
  await page.keyboard.press("s");
  await page.keyboard.press("1");
  await page.keyboard.press("t");
  await expect(page.getByText("This case is complete.")).toBeVisible();

  await page.keyboard.press("]");
  await expect(page.getByText("Case 2 of 2")).toBeVisible();
  await page.keyboard.press("c");
  await page.keyboard.press("t");
  await expect(page.getByText("2 complete · 0 remaining")).toBeVisible();

  await page.reload();
  await expect(page.getByText("Case 2 of 2")).toBeVisible();
  await expect(page.getByText("2 complete · 0 remaining")).toBeVisible();
  await expect(page.getByRole("combobox", { name: "Speed" })).toHaveValue("1.5");
  await page.getByRole("heading", { name: "Navigation" }).click();
  await page.keyboard.press("[");
  await expect(page.getByText("Case 1 of 2")).toBeVisible();

  const downloadEvent = page.waitForEvent("download");
  await page.getByRole("button", { name: "Export completed submission" }).click();
  const download = await downloadEvent;
  expect(download.suggestedFilename()).toBe("temporal-review-browser-test-batch.json");
  const downloadedPath = await download.path();
  if (!downloadedPath) throw new Error("browser did not retain the exported submission");
  const submission = JSON.parse(await readFile(downloadedPath, "utf8"));
  expect(submission).toMatchObject({
    batchId: "browser-test-batch",
    packageSha256: packageSHA256,
    reviewerId: "reviewer-browser",
    preparedAt: "2026-09-01T12:00:00Z",
  });
  expect(submission.answers).toEqual([
    {
      alias: reviewPackage.cases[0].alias,
      reviewerId: "reviewer-browser",
      unit: "standalone",
      role: "commercial",
      decisiveAtMs: 0,
    },
    {
      alias: reviewPackage.cases[1].alias,
      reviewerId: "reviewer-browser",
      unit: "compilation",
      decisiveAtMs: 0,
    },
  ]);
  expect(requests.filter((url) => url.startsWith("http://") || url.startsWith("https://"))).toEqual([]);
});
