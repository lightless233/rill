import { describe, expect, it } from "vitest";
import {
  isValidShareCustomHours,
  normalizeSharePasscodeInput,
} from "./share";

describe("isValidShareCustomHours", () => {
  it("accepts the inclusive server range", () => {
    expect(isValidShareCustomHours("1")).toBe(true);
    expect(isValidShareCustomHours("8760")).toBe(true);
  });

  it("rejects out-of-range, fractional and non-numeric values", () => {
    expect(isValidShareCustomHours("0")).toBe(false);
    expect(isValidShareCustomHours("8761")).toBe(false);
    expect(isValidShareCustomHours("1.5")).toBe(false);
    expect(isValidShareCustomHours("")).toBe(false);
    expect(isValidShareCustomHours("abc")).toBe(false);
  });
});

describe("normalizeSharePasscodeInput", () => {
  it("uppercases and keeps four alphanumeric characters", () => {
    expect(normalizeSharePasscodeInput("a7k2")).toBe("A7K2");
    expect(normalizeSharePasscodeInput("a7-k 2xyz")).toBe("A7K2");
  });

  it("rejects an incomplete code through length, not padding", () => {
    expect(normalizeSharePasscodeInput("a7")).toBe("A7");
    expect(normalizeSharePasscodeInput("")).toBe("");
  });
});
