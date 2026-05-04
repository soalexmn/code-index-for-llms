/** AuthForm renders a login form and handles submission. */
import React, { useState, useCallback } from "react";

export interface LoginProps {
  onSuccess: (token: string) => void;
  onError?: (message: string) => void;
  isLoading?: boolean;
}

interface FormState {
  email: string;
  password: string;
  error: string | null;
  submitting: boolean;
}

function useAuthForm(onSuccess: (token: string) => void, onError?: (msg: string) => void) {
  const [state, setState] = useState<FormState>({
    email: "",
    password: "",
    error: null,
    submitting: false,
  });

  const setField = useCallback(
    (field: keyof Pick<FormState, "email" | "password">) =>
      (e: React.ChangeEvent<HTMLInputElement>) =>
        setState(s => ({ ...s, [field]: e.target.value })),
    [],
  );

  const submit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      setState(s => ({ ...s, submitting: true, error: null }));
      try {
        const res = await fetch("/api/auth/login", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ email: state.email, password: state.password }),
        });
        if (!res.ok) {
          throw new Error(await res.text());
        }
        const { token } = await res.json();
        onSuccess(token);
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Login failed";
        setState(s => ({ ...s, error: msg, submitting: false }));
        onError?.(msg);
      }
    },
    [state.email, state.password, onSuccess, onError],
  );

  return { state, setField, submit };
}

export const AuthForm: React.FC<LoginProps> = ({ onSuccess, onError }) => {
  const { state, setField, submit } = useAuthForm(onSuccess, onError);

  return (
    <form className="auth-form" onSubmit={submit}>
      <h2>Sign In</h2>
      {state.error && <p className="auth-form__error">{state.error}</p>}
      <label>
        Email
        <input
          type="email"
          value={state.email}
          onChange={setField("email")}
          autoComplete="email"
          required
        />
      </label>
      <label>
        Password
        <input
          type="password"
          value={state.password}
          onChange={setField("password")}
          autoComplete="current-password"
          required
        />
      </label>
      <button type="submit" disabled={state.submitting}>
        {state.submitting ? "Signing in…" : "Sign In"}
      </button>
    </form>
  );
};

export default AuthForm;
