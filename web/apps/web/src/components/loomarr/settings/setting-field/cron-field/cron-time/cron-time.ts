// A schedule setting stores a six-field, seconds-leading cron. The household only ever means one
// of two shapes by it: a named preset the registry publishes, or "every day at a time". This reads
// and writes the second shape so the admin picks an hour and a minute instead of typing fields.
const DAILY = /^0 (\d{1,2}) (\d{1,2}) \* \* \*$/;

interface DailyTime {
  hour: number;
  minute: number;
}

const parseDaily = (cron: string): DailyTime | undefined => {
  const match = DAILY.exec(cron.trim());
  if (!match) return undefined;
  const minute = Number(match[1]);
  const hour = Number(match[2]);
  if (minute > 59 || hour > 23) return undefined;
  return { hour, minute };
};

const dailyCron = ({ hour, minute }: DailyTime): string => `0 ${minute} ${hour} * * *`;

export type { DailyTime };
export { dailyCron, parseDaily };
