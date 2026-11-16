import { API_BASE, api, parseAPIError } from "./client";
import type {
  APIResponse,
  ListResponse,
  PaginatedListResponse,
  LoginRequest,
  Group,
  Feed,
  Item,
  Bookmark,
  CreateGroupRequest,
  UpdateGroupRequest,
  ReorderGroupsRequest,
  CreateFeedRequest,
  UpdateFeedRequest,
  ValidateFeedRequest,
  ValidateFeedResponse,
  CreateBookmarkRequest,
  MarkAllItemsReadRequest,
  MarkAllItemsReadResponse,
  MarkItemsReadRequest,
  ListItemsParams,
  ListBookmarksParams,
  BatchCreateFeedsRequest,
  BatchCreateFeedsResponse,
  SearchResponse,
  OIDCStatusResponse,
  OIDCLoginResponse,
  AppInfoResponse,
  Settings,
  UpdateSettingsRequest,
  TranslateArticleRequest,
  TranslateArticleBlocksRequest,
  TranslationLookupResponse,
  TranslationTestRequest,
  TranslationTestResponse,
  SummarizeArticleRequest,
  SummaryLookupResponse,
  TranslateListItemsRequest,
  ListTranslationItemResponse,
  ListTranslationMeta,
  ListTranslationDone,
} from "./types";

// Session APIs
export const sessionAPI = {
  login: (data: LoginRequest) =>
    api.post<APIResponse<{ message: string }>>("/sessions", data),

  status: () => api.get<void>("/sessions"),

  logout: () => api.delete<void>("/sessions"),
};

// OIDC APIs
export const oidcAPI = {
  status: () => api.get<APIResponse<OIDCStatusResponse>>("/oidc/enabled"),

  login: () => api.get<APIResponse<OIDCLoginResponse>>("/oidc/login"),
};

// Group APIs
export const groupAPI = {
  list: () => api.get<ListResponse<Group>>("/groups"),

  get: (id: number) => api.get<APIResponse<Group>>(`/groups/${id}`),

  create: (data: CreateGroupRequest) =>
    api.post<APIResponse<Group>>("/groups", data),

  update: (id: number, data: UpdateGroupRequest) =>
    api.patch<APIResponse<Group>>(`/groups/${id}`, data),

  reorder: (data: ReorderGroupsRequest) =>
    api.patch<ListResponse<Group>>("/groups/-/order", data),

  delete: (id: number) => api.delete<void>(`/groups/${id}`),
};

// Feed APIs
export const feedAPI = {
  list: () => api.get<ListResponse<Feed>>("/feeds"),

  get: (id: number) => api.get<APIResponse<Feed>>(`/feeds/${id}`),

  create: (data: CreateFeedRequest) =>
    api.post<APIResponse<Feed>>("/feeds", data),

  update: (id: number, data: UpdateFeedRequest) =>
    api.patch<APIResponse<Feed>>(`/feeds/${id}`, data),

  delete: (id: number) => api.delete<void>(`/feeds/${id}`),

  validate: (data: ValidateFeedRequest) =>
    api.post<APIResponse<ValidateFeedResponse>>("/feeds/validate", data),

  refresh: () => api.post<void>("/feeds/refresh"),

  batchCreate: (data: BatchCreateFeedsRequest) =>
    api.post<APIResponse<BatchCreateFeedsResponse>>("/feeds/batch", data),
};

// Item APIs
export const itemAPI = {
  list: (params?: ListItemsParams) => {
    const query = new URLSearchParams();
    if (params?.feed_id) query.set("feed_id", params.feed_id.toString());
    if (params?.group_id) query.set("group_id", params.group_id.toString());
    if (params?.unread !== undefined)
      query.set("unread", params.unread.toString());
    if (params?.limit) query.set("limit", params.limit.toString());
    if (params?.before) query.set("before", params.before);
    if (params?.order_by) query.set("order_by", params.order_by);

    const queryString = query.toString();
    return api.get<PaginatedListResponse<Item>>(
      `/items${queryString ? `?${queryString}` : ""}`,
    );
  },

  get: (id: number) => api.get<APIResponse<Item>>(`/items/${id}`),

  markRead: (data: MarkItemsReadRequest) =>
    api.patch<void>("/items/-/read", data),

  markAllRead: (data: MarkAllItemsReadRequest) =>
    api.patch<APIResponse<MarkAllItemsReadResponse>>("/items/-/read-all", data),

  markUnread: (data: MarkItemsReadRequest) =>
    api.patch<void>("/items/-/unread", data),
};

// Bookmark APIs
export const bookmarkAPI = {
  list: (params: ListBookmarksParams = {}) => {
    const query = new URLSearchParams();
    if (params.feed_id) query.set("feed_id", params.feed_id.toString());
    if (params.group_id) query.set("group_id", params.group_id.toString());
    query.set("limit", (params.limit ?? 50).toString());
    if (params.before) query.set("before", params.before);
    return api.get<PaginatedListResponse<Bookmark>>(`/bookmarks?${query}`);
  },

  get: (id: number) => api.get<APIResponse<Bookmark>>(`/bookmarks/${id}`),

  create: (data: CreateBookmarkRequest) =>
    api.post<APIResponse<Bookmark>>("/bookmarks", data),

  delete: (id: number) => api.delete<void>(`/bookmarks/${id}`),
};

// Settings APIs
export const settingsAPI = {
  get: () => api.get<APIResponse<Settings>>("/settings"),

  update: (data: UpdateSettingsRequest) =>
    api.patch<APIResponse<Settings>>("/settings", data),
};

export interface TranslationStreamCallbacks {
  onMeta?: (meta: { total: number; model: string; cached: boolean }) => void;
  onSegment?: (segment: { key: string; text: string }) => void;
  onDone?: (done: { model: string; cached: boolean }) => void;
}

export interface SummaryStreamCallbacks {
  onMeta?: (meta: { model: string; cached: boolean }) => void;
  onDelta?: (delta: { text: string }) => void;
  onDone?: (done: { summary: string; model: string; cached: boolean }) => void;
}

export interface ListTranslationStreamCallbacks {
  onMeta?: (meta: ListTranslationMeta) => void;
  onItem?: (item: ListTranslationItemResponse) => void;
  onDone?: (done: ListTranslationDone) => void;
}

export interface ArticleBlockStreamCallbacks {
  onMeta?: (meta: { model: string; total: number }) => void;
  onSegment?: (segment: { key: string; text: string }) => void;
  onDone?: (done: { model: string }) => void;
}

async function streamSSEEvents(
  path: string,
  body: unknown,
  onEvent: (eventName: string, payload: Record<string, unknown>) => void,
  signal?: AbortSignal,
): Promise<void> {
  const response = await fetch(`${API_BASE}${path}`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
    signal,
  });

  if (!response.ok) {
    throw await parseAPIError(path, response);
  }
  if (!response.body) {
    throw new Error("Empty response stream");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";

  const dispatchEventBlock = (rawBlock: string) => {
    const normalized = rawBlock.replace(/\r\n/g, "\n");
    let eventName = "message";
    const dataLines: string[] = [];

    for (const line of normalized.split("\n")) {
      if (!line || line.startsWith(":")) continue;
      if (line.startsWith("event:")) {
        eventName = line.slice(6).trim();
      } else if (line.startsWith("data:")) {
        dataLines.push(line.slice(5).trimStart());
      }
    }

    if (dataLines.length === 0) return;
    const payload = JSON.parse(dataLines.join("\n")) as Record<
      string,
      unknown
    >;
    if (eventName === "error") {
      throw new Error(
        typeof payload.error === "string" ? payload.error : "Stream failed",
      );
    }
    onEvent(eventName, payload);
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      let boundary: number;
      while ((boundary = buffer.search(/\r?\n\r?\n/)) !== -1) {
        const match = buffer.slice(boundary).match(/^\r?\n\r?\n/);
        const sepLen = match ? match[0].length : 2;
        const block = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + sepLen);
        if (block.trim()) {
          dispatchEventBlock(block);
        }
      }
    }

    buffer += decoder.decode();
    if (buffer.trim()) {
      dispatchEventBlock(buffer);
    }
  } finally {
    reader.releaseLock();
  }
}

// Translation APIs
export const translationAPI = {
  lookup: (data: TranslateArticleRequest, signal?: AbortSignal) =>
    api.post<APIResponse<TranslationLookupResponse>>(
      "/translations/lookup",
      data,
      signal ? { signal } : undefined,
    ),

  test: (data: TranslationTestRequest) =>
    api.post<APIResponse<TranslationTestResponse>>("/translations/test", data),

  translateBlocks: (
    data: TranslateArticleBlocksRequest,
    callbacks: ArticleBlockStreamCallbacks,
    signal?: AbortSignal,
  ): Promise<void> =>
    streamSSEEvents(
      "/translations/blocks",
      data,
      (eventName, payload) => {
        if (eventName === "meta") {
          callbacks.onMeta?.(
            payload as unknown as { model: string; total: number },
          );
        } else if (eventName === "segment") {
          callbacks.onSegment?.(
            payload as unknown as { key: string; text: string },
          );
        } else if (eventName === "done") {
          callbacks.onDone?.(payload as unknown as { model: string });
        }
      },
      signal,
    ),

  translateItems: (
    data: TranslateListItemsRequest,
    callbacks: ListTranslationStreamCallbacks,
    signal?: AbortSignal,
  ): Promise<void> =>
    streamSSEEvents(
      "/translations/items",
      data,
      (eventName, payload) => {
        if (eventName === "meta") {
          callbacks.onMeta?.(payload as unknown as ListTranslationMeta);
        } else if (eventName === "item") {
          callbacks.onItem?.(
            payload as unknown as ListTranslationItemResponse,
          );
        } else if (eventName === "done") {
          callbacks.onDone?.(payload as unknown as ListTranslationDone);
        }
      },
      signal,
    ),

  stream: (
    data: TranslateArticleRequest,
    callbacks: TranslationStreamCallbacks,
    signal?: AbortSignal,
  ): Promise<void> =>
    streamSSEEvents(
      "/translations",
      data,
      (eventName, payload) => {
        if (eventName === "meta") {
          callbacks.onMeta?.(
            payload as { total: number; model: string; cached: boolean },
          );
        } else if (eventName === "segment") {
          callbacks.onSegment?.(payload as { key: string; text: string });
        } else if (eventName === "done") {
          callbacks.onDone?.(payload as { model: string; cached: boolean });
        }
      },
      signal,
    ),
};

// AI Summary APIs
export const summaryAPI = {
  lookup: (data: SummarizeArticleRequest, signal?: AbortSignal) =>
    api.post<APIResponse<SummaryLookupResponse>>(
      "/summaries/lookup",
      data,
      signal ? { signal } : undefined,
    ),

  stream: (
    data: SummarizeArticleRequest,
    callbacks: SummaryStreamCallbacks,
    signal?: AbortSignal,
  ): Promise<void> =>
    streamSSEEvents(
      "/summaries",
      data,
      (eventName, payload) => {
        if (eventName === "meta") {
          callbacks.onMeta?.(payload as { model: string; cached: boolean });
        } else if (eventName === "delta") {
          callbacks.onDelta?.(payload as { text: string });
        } else if (eventName === "done") {
          callbacks.onDone?.(
            payload as { summary: string; model: string; cached: boolean },
          );
        }
      },
      signal,
    ),
};

// Search APIs
export const searchAPI = {
  search: (q: string, limit = 10, signal?: AbortSignal) =>
    api.get<APIResponse<SearchResponse>>(
      `/search?q=${encodeURIComponent(q)}&limit=${limit}`,
      signal ? { signal } : undefined,
    ),
};

// App APIs
export const appAPI = {
  getInfo: () => api.get<APIResponse<AppInfoResponse>>("/app"),
};

export * from "./types";
export { APIError, setUnauthorizedCallback } from "./client";
