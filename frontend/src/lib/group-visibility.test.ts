import { describe, expect, it } from "vitest";
import { shouldHideDefaultGroup } from "./group-visibility";

describe("shouldHideDefaultGroup", () => {
  it("hides empty default group only when feeds query succeeded without error", () => {
    expect(
      shouldHideDefaultGroup({
        groupId: 1,
        unfilteredGroupFeedCount: 0,
        isFeedsSuccess: true,
        isFeedsError: false,
      }),
    ).toBe(true);
  });

  it("keeps default group visible when it contains feeds", () => {
    expect(
      shouldHideDefaultGroup({
        groupId: 1,
        unfilteredGroupFeedCount: 2,
        isFeedsSuccess: true,
        isFeedsError: false,
      }),
    ).toBe(false);
  });

  it("keeps default group visible while loading or in error state even with empty feeds", () => {
    expect(
      shouldHideDefaultGroup({
        groupId: 1,
        unfilteredGroupFeedCount: 0,
        isFeedsSuccess: false,
        isFeedsError: false,
      }),
    ).toBe(false);

    expect(
      shouldHideDefaultGroup({
        groupId: 1,
        unfilteredGroupFeedCount: 0,
        isFeedsSuccess: false,
        isFeedsError: true,
      }),
    ).toBe(false);
  });

  it("never hides non-default groups when empty", () => {
    expect(
      shouldHideDefaultGroup({
        groupId: 2,
        unfilteredGroupFeedCount: 0,
        isFeedsSuccess: true,
        isFeedsError: false,
      }),
    ).toBe(false);
  });
});
