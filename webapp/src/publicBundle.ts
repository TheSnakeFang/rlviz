export const publicBundleURLLimit = 2_048;

export interface PublicBundleRequest {
  url: URL;
  digest: string;
  name: string;
}

export type PublicBundleHandoff =
  | { kind: "none" }
  | { kind: "invalid"; message: string }
  | { kind: "ready"; request: PublicBundleRequest };

export function parsePublicBundleHandoff(search: string): PublicBundleHandoff {
  const params = new URLSearchParams(search);
  if (params.getAll("bundle").length > 1 || params.getAll("sha256").length > 1) {
    return { kind: "invalid", message: "Shared bundle links cannot repeat bundle or sha256 parameters." };
  }
  const rawURL = params.get("bundle");
  const rawDigest = params.get("sha256");
  if (rawURL === null && rawDigest === null) return { kind: "none" };
  if (!rawURL || !rawDigest) return { kind: "invalid", message: "Shared bundle links require both bundle and sha256 parameters." };
  if (rawURL.length > publicBundleURLLimit) return { kind: "invalid", message: "The shared bundle URL is too long." };
  if (!/^[0-9a-fA-F]{64}$/.test(rawDigest)) return { kind: "invalid", message: "The shared bundle SHA-256 must be exactly 64 hexadecimal characters." };

  let url: URL;
  try { url = new URL(rawURL); }
  catch { return { kind: "invalid", message: "The shared bundle URL is invalid." }; }
  if (url.protocol !== "https:") return { kind: "invalid", message: "Shared bundles must use a public HTTPS URL." };
  if (url.username || url.password) return { kind: "invalid", message: "Shared bundle URLs cannot contain credentials." };
  if (url.search || url.hash) return { kind: "invalid", message: "Shared bundle URLs cannot contain query parameters or fragments." };
  let name: string;
  try { name = decodeURIComponent(url.pathname.split("/").pop() || "shared.rlviz"); }
  catch { return { kind: "invalid", message: "The shared bundle filename is invalid." }; }
  if (name.length > 255 || /[\\/\u0000-\u001f\u007f]/.test(name)) return { kind: "invalid", message: "The shared bundle filename is invalid." };
  if (!name.toLowerCase().endsWith(".rlviz")) return { kind: "invalid", message: "Shared bundle URLs must identify an .rlviz file." };
  return { kind: "ready", request: { url, digest: rawDigest.toLowerCase(), name } };
}

export async function sha256(bytes: Uint8Array): Promise<string> {
  const hash = await crypto.subtle.digest("SHA-256", bytes);
  return [...new Uint8Array(hash)].map((value) => value.toString(16).padStart(2, "0")).join("");
}

export async function fetchVerifiedPublicBundle(request: PublicBundleRequest, maxBytes: number): Promise<Uint8Array> {
  const response = await fetch(request.url, {
    cache: "no-store",
    credentials: "omit",
    redirect: "error",
    referrerPolicy: "no-referrer",
  });
  if (!response.ok) throw new Error(`Could not fetch shared bundle (HTTP ${response.status})`);
  if (response.redirected) throw new Error("Shared bundle redirects are not allowed");
  const declaredSize = response.headers.get("content-length");
  if (declaredSize !== null && (!/^\d+$/.test(declaredSize) || Number(declaredSize) > maxBytes)) {
    throw new Error(`Shared bundle exceeds the ${maxBytes / 1024 / 1024} MiB browser maximum`);
  }
  const bytes = await readBounded(response, maxBytes);
  const actual = await sha256(bytes);
  if (actual !== request.digest) throw new Error(`Shared bundle SHA-256 mismatch: expected ${request.digest}, received ${actual}`);
  return bytes;
}

async function readBounded(response: Response, maxBytes: number): Promise<Uint8Array> {
  if (!response.body) {
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (bytes.byteLength > maxBytes) throw new Error(`Shared bundle exceeds the ${maxBytes / 1024 / 1024} MiB browser maximum`);
    return bytes;
  }
  const reader = response.body.getReader();
  const chunks: Uint8Array[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > maxBytes) {
        await reader.cancel();
        throw new Error(`Shared bundle exceeds the ${maxBytes / 1024 / 1024} MiB browser maximum`);
      }
      chunks.push(value);
    }
  } finally { reader.releaseLock(); }
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.byteLength; }
  return bytes;
}
