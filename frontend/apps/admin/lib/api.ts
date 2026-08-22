export type Me = {
  email: string;
  displayed_name: string;
  global_admin: boolean;
  enabled: boolean;
};

const API = "/api/v1";

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error((body as { error?: string }).error || res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const apiPost = (path: string, body: unknown) =>
  api(path, { method: "POST", body: JSON.stringify(body) });

export const apiPut = (path: string, body: unknown) =>
  api(path, { method: "PUT", body: JSON.stringify(body) });

export const apiDelete = (path: string) => api(path, { method: "DELETE" });

export async function login(email: string, pw: string) {
  return api<Me>("/sso/login", {
    method: "POST",
    body: JSON.stringify({ email, pw }),
  });
}

export async function logout() {
  return api("/sso/logout", { method: "POST" });
}

export async function me(): Promise<Me> {
  return api<Me>("/sso/me");
}
