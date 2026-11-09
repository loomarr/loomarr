import type { BackupEntry } from "@loomarr/api/models/backupEntry";

const localDay = (writtenAtSeconds: number): number => {
  const written = new Date(writtenAtSeconds * 1000);
  return Date.UTC(written.getFullYear(), written.getMonth(), written.getDate());
};

const DAY_MS = 86_400_000;

// How many calendar days between the oldest and newest backup on disk have no backup at all.
// The series is judged only between its own ends: the day after the newest is not "missing", it
// just has not run yet, and nothing is known about the days before the oldest because retention
// prunes them. Two backups on one day (a manual one beside the nightly) count as one night.
const missingNights = (backups: readonly Pick<BackupEntry, "writtenAt">[]): number => {
  if (backups.length < 2) return 0;
  const days = new Set(backups.map((backup) => localDay(backup.writtenAt)));
  const ordered = [...days].sort((a, b) => a - b);
  const first = ordered[0] ?? 0;
  const last = ordered[ordered.length - 1] ?? 0;
  return Math.round((last - first) / DAY_MS) + 1 - days.size;
};

export { missingNights };
