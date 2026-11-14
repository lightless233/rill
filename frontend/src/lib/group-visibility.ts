export const DEFAULT_GROUP_ID = 1;

export interface GroupVisibilityInput {
  groupId: number;
  unfilteredGroupFeedCount: number;
  isFeedsSuccess: boolean;
  isFeedsError: boolean;
}

/**
 * Hides the system Default group (id === 1, even if renamed) only once the
 * unfiltered feeds query has succeeded and confirmed it contains zero feeds.
 */
export function shouldHideDefaultGroup(input: GroupVisibilityInput): boolean {
  if (input.groupId !== DEFAULT_GROUP_ID) {
    return false;
  }
  if (!input.isFeedsSuccess || input.isFeedsError) {
    return false;
  }
  return input.unfilteredGroupFeedCount === 0;
}
