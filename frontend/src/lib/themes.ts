export const themeOptions = [
  "light",
  "dark",
  "system",
  "sage",
  "parchment",
  "stone",
  "night",
] as const;

export type AppTheme = (typeof themeOptions)[number];

export function isTheme(value: string): value is AppTheme {
  return themeOptions.some((theme) => theme === value);
}
