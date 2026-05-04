/** Core domain types shared across the frontend. */

export type UserId = string;

export enum Role {
  Admin = "admin",
  Editor = "editor",
  Viewer = "viewer",
  Guest = "guest",
}

export enum Permission {
  Read = "read",
  Write = "write",
  Delete = "delete",
  ManageUsers = "manage_users",
  ViewLogs = "view_logs",
}

/** User represents an authenticated application user. */
export interface User {
  id: UserId;
  email: string;
  role: Role;
  displayName: string;
  isActive: boolean;
  createdAt: string; // ISO 8601
}

/** Generic API response envelope used by all endpoints. */
export interface ApiResponse<T> {
  data: T;
  error: string | null;
  status: number;
  requestId: string;
}

/** Paginated list response for collection endpoints. */
export interface PaginatedResponse<T> extends ApiResponse<T[]> {
  page: number;
  pageSize: number;
  totalCount: number;
}

/** Partial user fields used in create/update requests. */
export type UserInput = Omit<User, "id" | "createdAt">;
