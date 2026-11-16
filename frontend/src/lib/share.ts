export const SHARE_CUSTOM_HOURS_MIN = 1;
export const SHARE_CUSTOM_HOURS_MAX = 365 * 24;

export function isValidShareCustomHours(value: string): boolean {
  const hours = Number(value);
  return (
    Number.isInteger(hours) &&
    hours >= SHARE_CUSTOM_HOURS_MIN &&
    hours <= SHARE_CUSTOM_HOURS_MAX
  );
}

export function normalizeSharePasscodeInput(value: string): string {
  return value
    .replace(/[^a-zA-Z0-9]/g, "")
    .toUpperCase()
    .slice(0, 4);
}
