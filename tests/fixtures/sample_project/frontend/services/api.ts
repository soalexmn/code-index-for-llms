/** HTTP client for the user management API. */
import { User, ApiResponse, PaginatedResponse, UserId, UserInput } from "../types/models";

/** ApiError is thrown when the server returns a non-2xx status. */
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: string,
    message?: string,
  ) {
    super(message ?? `HTTP ${status}`);
    this.name = "ApiError";
  }
}

/** ApiClient wraps fetch with auth headers and JSON parsing. */
export class ApiClient {
  private baseUrl: string;
  private token: string | null = null;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
  }

  setToken(token: string): void {
    this.token = token;
  }

  clearToken(): void {
    this.token = null;
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: HeadersInit = { "Content-Type": "application/json" };
    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }
    const res = await fetch(`${this.baseUrl}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
    const text = await res.text();
    if (!res.ok) {
      throw new ApiError(res.status, text);
    }
    return JSON.parse(text) as T;
  }

  async fetchUser(id: UserId): Promise<ApiResponse<User>> {
    return this.request<ApiResponse<User>>("GET", `/users/${id}`);
  }

  async createUser(payload: UserInput): Promise<ApiResponse<User>> {
    return this.request<ApiResponse<User>>("POST", "/users", payload);
  }

  async deleteUser(id: UserId): Promise<void> {
    await this.request<void>("DELETE", `/users/${id}`);
  }

  async listUsers(page = 1, pageSize = 20): Promise<PaginatedResponse<User>> {
    return this.request<PaginatedResponse<User>>("GET", `/users?page=${page}&page_size=${pageSize}`);
  }
}

/** Module-level convenience wrappers using a shared default client. */
let _defaultClient: ApiClient | null = null;

export function getDefaultClient(): ApiClient {
  if (!_defaultClient) {
    throw new Error("ApiClient not initialised; call initClient() first");
  }
  return _defaultClient;
}

export function initClient(baseUrl: string): ApiClient {
  _defaultClient = new ApiClient(baseUrl);
  return _defaultClient;
}

export async function fetchUser(id: UserId): Promise<User> {
  const res = await getDefaultClient().fetchUser(id);
  return res.data;
}

export async function createUser(payload: UserInput): Promise<User> {
  const res = await getDefaultClient().createUser(payload);
  return res.data;
}
