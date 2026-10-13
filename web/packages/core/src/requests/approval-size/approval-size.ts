import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { SizeConfidence } from "@loomarr/api/models/sizeConfidence";
import { formatBytes } from "../../format";

// What approving costs on disk, worded so it never claims more than the server knows (#1817,
// #1876). Shared by web and native, which render the same consequence summary.
//
// ⚠ Three honesty rules, each one a way this summary could otherwise lie:
//   1. An unknown size is ABSENT on the wire (both fields), never 0. It renders as
//      "estimate unavailable", never "0 B" and never by leaving the line out — a missing
//      line reads as "nothing to download".
//   2. An estimate says so twice, "~" and "(estimated)", because one of the two is easy to miss.
//   3. A sum over SOME of the items is never shown as a total. It names how many items it
//      covers and how many are unknown.

// The size fields any sized item carries. ProposalItem is the only one today (#1885); filler
// pull rows will reuse SizeConfidence when a pull producer returns (beta.11).
type SizedItem = Pick<ProposalItem, "sizeBytes" | "sizeConfidence">;

interface ApprovalSize {
  // Sum over the items whose size is known. Meaningless on its own when `known < total`.
  bytes: number;
  known: number;
  total: number;
  // At least one known item is an estimate, so the sum is one too.
  estimated: boolean;
}

const SIZE_UNAVAILABLE = "estimate unavailable";

// A size the server stated. ⚠ A non-positive byte count is treated as unknown rather than
// trusted: the contract says absent, never 0, and "0 B" is exactly the lie rule 1 forbids.
const knownBytes = (item: SizedItem): number | null =>
  typeof item.sizeBytes === "number" && Number.isFinite(item.sizeBytes) && item.sizeBytes > 0
    ? item.sizeBytes
    : null;

// Bytes without a confidence are not exact as far as anyone can prove, so they count as an
// estimate rather than borrowing the stronger claim.
const isExact = (item: SizedItem): boolean => item.sizeConfidence === SizeConfidence.exact;

const formatSize = (bytes: number, estimated: boolean): string =>
  estimated ? `~${formatBytes(bytes)} (estimated)` : formatBytes(bytes);

// One item: "2.1 GB" · "~2.1 GB (estimated)" · "estimate unavailable".
const itemSizeLabel = (item: SizedItem): string => {
  const bytes = knownBytes(item);
  return bytes === null ? SIZE_UNAVAILABLE : formatSize(bytes, !isExact(item));
};

const aggregateSize = (items: readonly SizedItem[]): ApprovalSize => {
  const size: ApprovalSize = { bytes: 0, known: 0, total: items.length, estimated: false };
  for (const item of items) {
    const bytes = knownBytes(item);
    if (bytes === null) continue;
    size.bytes += bytes;
    size.known += 1;
    size.estimated ||= !isExact(item);
  }
  return size;
};

// Several items, summed honestly:
//   every size known  → "8.4 GB" (or "~8.4 GB (estimated)")
//   some known        → "8.4 GB for 4 of 14 titles · 10 unknown"
//   none known        → "estimate unavailable"
const aggregateSizeLabel = (size: ApprovalSize, plural: string): string => {
  if (size.known === 0) return SIZE_UNAVAILABLE;
  const sum = formatSize(size.bytes, size.estimated);
  if (size.known === size.total) return sum;
  return `${sum} for ${size.known} of ${size.total} ${plural} · ${size.total - size.known} unknown`;
};

// The consequence line itself, as every surface prints it.
const approvalSizeLine = (items: readonly SizedItem[], plural: string): string =>
  `Size: ${aggregateSizeLabel(aggregateSize(items), plural)}`;

export type { ApprovalSize, SizedItem };
export { aggregateSize, aggregateSizeLabel, approvalSizeLine, itemSizeLabel, SIZE_UNAVAILABLE };
