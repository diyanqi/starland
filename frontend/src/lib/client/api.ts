export interface Photo {
  id: number;
  url: string;
  caption: string;
  nickname: string;
  likes: number;
  liked: boolean;
  createdAt: string;
}

export interface Message {
  id: number;
  content: string;
  type: MessageType;
  nickname: string;
  replyCount: number;
  createdAt: string;
}

export interface Reply {
  id: number;
  messageId: number;
  content: string;
  nickname: string;
  createdAt: string;
}

export type MessageType = 'praise' | 'rant' | 'thought';

export interface PageResult<T> {
  items: T[];
  nextCursor: number;
}

const CLIENT_KEY = 'sgi:client-id';

function createClientId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  // 局域网 http 访问时不是 secure context，randomUUID 可能不可用。
  return `c-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

// 用于点赞幂等去重的浏览器标识。清除浏览器数据后会重新生成。
export function getClientId(): string {
  try {
    let id = localStorage.getItem(CLIENT_KEY);
    if (!id) {
      id = createClientId();
      localStorage.setItem(CLIENT_KEY, id);
    }
    return id;
  } catch {
    return 'anonymous-browser';
  }
}

export class ApiError extends Error {
  readonly code: string;
  readonly status: number;

  constructor(message: string, code: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
  }
}

async function parse<T>(response: Response): Promise<T> {
  if (response.status === 204) return undefined as T;

  const isJSON = response.headers.get('content-type')?.includes('application/json');
  const payload = isJSON ? await response.json().catch(() => null) : null;

  if (!response.ok) {
    throw new ApiError(
      payload?.error?.message ?? `请求失败（${response.status}）`,
      payload?.error?.code ?? 'http_error',
      response.status,
    );
  }
  return (payload?.data ?? payload) as T;
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('X-Client-Id', getClientId());
  if (typeof init.body === 'string') {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(path, { ...init, headers });
  return parse<T>(response);
}

export async function adminApi<T>(
  path: string,
  token: string,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('X-Admin-Token', token);
  if (typeof init.body === 'string') {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(path, { ...init, headers });
  return parse<T>(response);
}
