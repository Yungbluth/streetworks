
import { DataError, type Detail } from "./data";

export const API_BASE: string = import.meta.env.VITE_API || "http://127.0.0.1:8081";

export interface PermitHit {
  id: number;
  place: number;
  year: number;
  cat: number;
  number: string;
  snippet: string;
}

export interface Api {
  // full records for some permits, by index
  records(ids: number[]): Promise<Map<number, Detail>>;
  byNumber(n: string): Promise<{ id: number; place: number; permit: Detail }>;
  search(q: string, signal?: AbortSignal): Promise<PermitHit[]>;
}

const CHUNK = 2000; // the most ids the server takes in one lookup
const TIMEOUT_MS = 10_000;
const SEARCH_HITS = 8; // as many as the search list shows

export function apiClient(base: string, built: string, fetchFn: typeof fetch = fetch): Api {
  const root = base.replace(/\/+$/, "");
  const known = new Map<number, Detail | null>();
  const asking = new Map<number, Promise<void>>();

  async function call<T extends { built: string }>(path: string, init: RequestInit = {}): Promise<T> {
    const ctrl = new AbortController();
    const caller = init.signal;
    const cancel = () => ctrl.abort(caller?.reason);
    if (caller?.aborted) cancel();
    caller?.addEventListener("abort", cancel, { once: true });
    let timer: ReturnType<typeof setTimeout> | undefined;
    const timeout = new Promise<never>((_, reject) => {
      timer = setTimeout(() => {
        ctrl.abort();
        reject(new DataError("The permit server didn't answer in time. It may be starting up; try again."));
      }, TIMEOUT_MS);
    });
    try {
      return await Promise.race([send<T>(root + path, { ...init, signal: ctrl.signal }, caller), timeout]);
    } finally {
      clearTimeout(timer);
      caller?.removeEventListener("abort", cancel);
    }
  }

  async function send<T extends { built: string }>(url: string, init: RequestInit, caller?: AbortSignal | null): Promise<T> {
    let res: Response;
    try {
      res = await fetchFn(url, init);
    } catch (e) {
      if (caller?.aborted) throw e; // the caller cancelled it, which isn't a failure
      throw new DataError("Couldn't reach the permit server.");
    }
    let body: unknown;
    try {
      body = await res.json();
    } catch (e) {
      if (caller?.aborted) throw e;
      body = undefined;
    }
    if (!res.ok) {
      const message = (body as { message?: unknown } | undefined)?.message;
      throw new DataError(typeof message === "string" && message ? message : `The permit server said ${res.status}.`, res.status);
    }
    if (typeof body !== "object" || body === null) {
      throw new DataError("The permit server sent an answer this page can't read.");
    }
    if ((body as { built?: unknown }).built !== built) {
      throw new DataError("The permit data was updated while this page was open. Reload the page to see it.");
    }
    return body as T;
  }

  async function lookup(ids: number[]) {
    const res = await call<{ built: string; permits: Record<string, Detail> }>("/v1/permits/lookup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ids }),
    });
    for (const id of ids) known.set(id, res.permits[String(id)] ?? null);
  }

  return {
    async records(ids) {
      const waits = new Set<Promise<void>>();
      const missing: number[] = [];
      for (const id of new Set(ids)) {
        if (known.has(id)) continue;
        const p = asking.get(id);
        if (p) waits.add(p);
        else missing.push(id);
      }
      for (let i = 0; i < missing.length; i += CHUNK) {
        const chunk = missing.slice(i, i + CHUNK);
        const p = lookup(chunk).finally(() => chunk.forEach((id) => asking.delete(id)));
        chunk.forEach((id) => asking.set(id, p));
        waits.add(p);
      }
      await Promise.all(waits);
      const out = new Map<number, Detail>();
      for (const id of ids) {
        const d = known.get(id);
        if (d) out.set(id, d);
      }
      return out;
    },

    // GETs carry the page's build as v
    async byNumber(n) {
      const { id, place, permit } = await call<{ built: string; id: number; place: number; permit: Detail }>(
        `/v1/permits/${encodeURIComponent(n)}?${new URLSearchParams({ v: built })}`,
      );
      return { id, place, permit };
    },

    async search(q, signal) {
      const res = await call<{ built: string; hits: PermitHit[] }>(`/v1/search?${new URLSearchParams({ q, limit: String(SEARCH_HITS), v: built })}`, { signal });
      return res.hits;
    },
  };
}
