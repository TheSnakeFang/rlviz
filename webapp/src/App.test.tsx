import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("./wasm", () => ({
  limits: vi.fn(async () => ({ maxRecommendedBytes: 32 << 20 })),
  parseTrace: vi.fn(async () => ({ collection: {}, collection_id: "sample" })),
}));
vi.mock("./provider", () => ({ createInMemoryProvider: vi.fn(() => ({ kind: "sample" })) }));
vi.mock("../../web/src/App", () => ({ App: ({ setup }: { setup: { mode: string; status: string; selectedSample: string } }) => <main>sample viewer ready<span>{setup.mode}</span><span>{setup.status}</span><span>{setup.selectedSample}</span></main> }));

import { BrowserApp } from "./App";
import { parseTrace } from "./wasm";

describe("browser startup", () => {
  beforeEach(() => {
    const values = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      clear: () => values.clear(),
      getItem: (key: string) => values.get(key) ?? null,
      removeItem: (key: string) => values.delete(key),
      setItem: (key: string, value: string) => values.set(key, value),
    });
  });
  afterEach(() => { cleanup(); history.replaceState(null, "", "/"); vi.unstubAllGlobals(); });

  it("opens the reviewed Terminal-Bench cohort without an initial click", async () => {
    const fetch = vi.fn(async (_input: RequestInfo | URL) => new Response(new Uint8Array([1, 2, 3]), { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    render(<BrowserApp />);

    expect(await screen.findByText("sample viewer ready")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(String(fetch.mock.calls[0][0])).toContain("terminal-bench-2-showcase");
    expect(screen.getByText("browser")).toBeInTheDocument();
    expect(screen.getByText(/terminal-bench-2-showcase\.ndjson is open/)).toBeInTheDocument();
    expect(screen.getByText("terminal-bench-2-showcase.ndjson")).toBeInTheDocument();
    expect(screen.queryByRole("contentinfo")).not.toBeInTheDocument();
  });

  it("never paints the landing page while the bundled viewer initializes", async () => {
    let finish: ((value: Awaited<ReturnType<typeof parseTrace>>) => void) | undefined;
    vi.mocked(parseTrace).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
    vi.stubGlobal("fetch", vi.fn(async () => new Response(new Uint8Array([1, 2, 3]), { status: 200 })));
    render(<BrowserApp />);

    expect(screen.getByRole("status", { name: "Loading RLViz" })).toBeInTheDocument();
    expect(screen.queryByText("Inspect agent rollouts locally.")).not.toBeInTheDocument();
    await vi.waitFor(() => expect(finish).toBeTypeOf("function"));
    await act(async () => finish?.({ collection: {}, collection_id: "sample" } as Awaited<ReturnType<typeof parseTrace>>));
    expect(await screen.findByText("sample viewer ready")).toBeInTheDocument();
    expect(screen.queryByText("Inspect agent rollouts locally.")).not.toBeInTheDocument();
  });

  it("does not fetch a digest-pinned shared bundle before confirmation", async () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    history.replaceState(null, "", `/?bundle=${encodeURIComponent("https://bundles.example/reviewed.rlviz")}&sha256=${"a".repeat(64)}`);
    render(<BrowserApp />);

    expect(await screen.findByRole("region", { name: "Shared bundle confirmation" })).toBeInTheDocument();
    expect(screen.getByText("https://bundles.example/reviewed.rlviz")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Open shared trajectory" })).toBeInTheDocument();
    expect(screen.getByText("Nothing is uploaded.", { exact: false })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Verify and open" })).toBeInTheDocument();
    expect(screen.queryByText("Inspect agent rollouts locally.")).not.toBeInTheDocument();
    expect(document.querySelector(".kicker, .privacy-proof, .example-actions")).toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("drops source-specific saved state after a shared bundle is verified", async () => {
    const staleWorkspace = JSON.stringify({ version: 3, lanes: [{ sourceId: "sample", trajectoryId: "checkout-rollout-02", band: "focus" }] });
    localStorage.setItem("rlviz.workspace.v6", staleWorkspace);
    const digest = "039058c6f2c0cb492c533b0a4d14ef77cc0f78abccced5287d84a1a2011cfb81";
    history.replaceState(null, "", `/?bundle=${encodeURIComponent("https://bundles.example/reviewed.rlviz")}&sha256=${digest}&workspace=${encodeURIComponent(staleWorkspace)}`);
    vi.stubGlobal("fetch", vi.fn(async () => new Response(new Uint8Array([1, 2, 3]), { status: 200 })));
    render(<BrowserApp />);

    await userEvent.click(await screen.findByRole("button", { name: "Verify and open" }));

    expect(await screen.findByText("sample viewer ready")).toBeInTheDocument();
    expect(localStorage.getItem("rlviz.workspace.v6")).toBeNull();
    expect(window.location.search).toBe("");
  });
});
