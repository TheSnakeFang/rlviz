import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchVerifiedPublicBundle, parsePublicBundleHandoff, sha256 } from "./publicBundle";

describe("public bundle handoff", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("accepts only a digest-pinned public HTTPS .rlviz URL", () => {
    const digest = "a".repeat(64);
    const result = parsePublicBundleHandoff(`?bundle=${encodeURIComponent("https://bundles.example/reviewed.rlviz")}&sha256=${digest.toUpperCase()}`);
    expect(result).toMatchObject({ kind: "ready", request: { digest, name: "reviewed.rlviz" } });
  });

  it.each([
    ["missing digest", "?bundle=https%3A%2F%2Fbundles.example%2Fa.rlviz"],
    ["plain HTTP", `?bundle=${encodeURIComponent("http://bundles.example/a.rlviz")}&sha256=${"a".repeat(64)}`],
    ["credentials", `?bundle=${encodeURIComponent("https://secret@bundles.example/a.rlviz")}&sha256=${"a".repeat(64)}`],
    ["query tokens", `?bundle=${encodeURIComponent("https://bundles.example/a.rlviz?token=secret")}&sha256=${"a".repeat(64)}`],
    ["encoded path separator", `?bundle=${encodeURIComponent("https://bundles.example/a%2Fb.rlviz")}&sha256=${"a".repeat(64)}`],
    ["wrong extension", `?bundle=${encodeURIComponent("https://bundles.example/a.zip")}&sha256=${"a".repeat(64)}`],
    ["duplicate digest", `?bundle=${encodeURIComponent("https://bundles.example/a.rlviz")}&sha256=${"a".repeat(64)}&sha256=${"b".repeat(64)}`],
  ])("rejects %s", (_label, search) => expect(parsePublicBundleHandoff(search).kind).toBe("invalid"));

  it("verifies the downloaded bytes before returning them", async () => {
    const bytes = new Uint8Array([1, 2, 3]);
    const digest = await sha256(bytes);
    const fetch = vi.fn(async () => new Response(bytes, { status: 200, headers: { "content-length": "3" } }));
    vi.stubGlobal("fetch", fetch);
    const handoff = parsePublicBundleHandoff(`?bundle=https%3A%2F%2Fbundles.example%2Fa.rlviz&sha256=${digest}`);
    if (handoff.kind !== "ready") throw new Error("expected ready handoff");
    await expect(fetchVerifiedPublicBundle(handoff.request, 100)).resolves.toEqual(bytes);
    expect(fetch).toHaveBeenCalledWith(handoff.request.url, expect.objectContaining({ credentials: "omit", redirect: "error", referrerPolicy: "no-referrer" }));
  });

  it("rejects a digest mismatch", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(new Uint8Array([1, 2, 3]), { status: 200 })));
    const handoff = parsePublicBundleHandoff(`?bundle=https%3A%2F%2Fbundles.example%2Fa.rlviz&sha256=${"0".repeat(64)}`);
    if (handoff.kind !== "ready") throw new Error("expected ready handoff");
    await expect(fetchVerifiedPublicBundle(handoff.request, 100)).rejects.toThrow("SHA-256 mismatch");
  });

  it("stops reading a response once it crosses the browser limit", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(new Uint8Array([1, 2, 3, 4]), { status: 200 })));
    const handoff = parsePublicBundleHandoff(`?bundle=https%3A%2F%2Fbundles.example%2Fa.rlviz&sha256=${"0".repeat(64)}`);
    if (handoff.kind !== "ready") throw new Error("expected ready handoff");
    await expect(fetchVerifiedPublicBundle(handoff.request, 3)).rejects.toThrow("browser maximum");
  });
});
