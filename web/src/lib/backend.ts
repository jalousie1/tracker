import "server-only";

export type BackendFetchError = {
  status: number;
  bodyText: string;
};

function backendBaseUrl() {
  const raw = process.env.BACKEND_URL || "http://localhost:8080";
  return raw.replace(/\/+$/, "");
}

function mergeSignals(a: AbortSignal | undefined, b: AbortSignal): AbortSignal {
  if (!a) return b;
  // Node 20+ / browsers modernos
  const anyFn = (AbortSignal as unknown as { any?: (signals: AbortSignal[]) => AbortSignal }).any;
  if (typeof anyFn === "function") return anyFn([a, b]);

  // fallback: propaga abort do signal externo para o controller interno
  if (a.aborted) {
    (b as unknown as AbortSignal & { throwIfAborted?: () => void }).throwIfAborted?.();
    return b;
  }
  a.addEventListener(
    "abort",
    () => {
      // nada a fazer aqui; o fetch abaixo usa o signal interno
      // e o abort será acionado via controller.abort() onde for criado
    },
    { once: true },
  );
  return b;
}

async function fetchWithTimeout(url: string, init: RequestInit | undefined, timeoutMs: number) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(new Error("timeout")), timeoutMs);
  try {
    const signal = mergeSignals(init?.signal ?? undefined, controller.signal);
    // Se existir signal externo, precisamos abortar o controller interno quando ele abortar
    const externalSignal = init?.signal;
    if (externalSignal) {
      if (externalSignal.aborted) controller.abort(externalSignal.reason);
      else externalSignal.addEventListener("abort", () => controller.abort(externalSignal.reason), { once: true });
    }
    return await fetch(url, { ...init, signal });
  } finally {
    clearTimeout(timeout);
  }
}

export async function backendFetchJson<T>(path: string, init?: RequestInit): Promise<T> {
  // Adicionar prefixo /api/v1 se não estiver presente
  let apiPath = path;
  if (!apiPath.startsWith("/api/v1")) {
    apiPath = `/api/v1${apiPath.startsWith("/") ? "" : "/"}${apiPath}`;
  }
  const url = `${backendBaseUrl()}${apiPath}`;

  // O backend do Go usa timeout de ~10s. No Next/Node, fetch sem timeout pode ficar preso
  // e parecer "compilando/carregando infinito" no dev. Mantemos um timeout um pouco maior.
  const res = await fetchWithTimeout(
    url,
    {
      ...init,
      headers: {
        accept: "application/json",
        ...(init?.headers || {}),
      },
      cache: "no-store",
    },
    15_000,
  );

  const bodyText = await res.text();
  if (!res.ok) {
    const err: BackendFetchError = { status: res.status, bodyText };
    throw err;
  }

  return JSON.parse(bodyText) as T;
}


