/** Authentication service: login, logout, token refresh. */
import { User } from "../types/models";

const TOKEN_KEY = "auth_token";
const USER_KEY = "auth_user";

/** AuthState holds the currently authenticated user and token. */
export interface AuthState {
  user: User | null;
  token: string | null;
}

/** AuthService manages the session lifecycle. */
export class AuthService {
  private _state: AuthState = { user: null, token: null };
  private _listeners: Array<(state: AuthState) => void> = [];

  constructor(private readonly apiBase: string) {}

  async login(email: string, password: string): Promise<AuthState> {
    const res = await fetch(`${this.apiBase}/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    if (!res.ok) {
      throw new Error(`Login failed: ${res.status}`);
    }
    const { token, user } = await res.json();
    this._state = { user, token };
    localStorage.setItem(TOKEN_KEY, token);
    localStorage.setItem(USER_KEY, JSON.stringify(user));
    this._notify();
    return this._state;
  }

  async logout(): Promise<void> {
    const token = this._state.token;
    if (token) {
      await fetch(`${this.apiBase}/auth/logout`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {});
    }
    this._state = { user: null, token: null };
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
    this._notify();
  }

  async refreshToken(): Promise<string> {
    const current = this._state.token ?? getStoredToken();
    if (!current) {
      throw new Error("no token to refresh");
    }
    const res = await fetch(`${this.apiBase}/auth/refresh`, {
      method: "POST",
      headers: { Authorization: `Bearer ${current}` },
    });
    if (!res.ok) {
      throw new Error("token refresh failed");
    }
    const { token } = await res.json();
    this._state = { ...this._state, token };
    localStorage.setItem(TOKEN_KEY, token);
    this._notify();
    return token;
  }

  isAuthenticated(): boolean {
    return this._state.token !== null;
  }

  getState(): AuthState {
    return { ...this._state };
  }

  subscribe(listener: (state: AuthState) => void): () => void {
    this._listeners.push(listener);
    return () => {
      this._listeners = this._listeners.filter(l => l !== listener);
    };
  }

  private _notify(): void {
    this._listeners.forEach(l => l(this._state));
  }
}

/** Return the token stored in localStorage, or null if absent. */
export function getStoredToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

/** Return the user stored in localStorage, or null if absent. */
export function getStoredUser(): User | null {
  const raw = localStorage.getItem(USER_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as User;
  } catch {
    return null;
  }
}
