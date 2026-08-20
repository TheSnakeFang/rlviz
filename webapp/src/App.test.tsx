import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("./wasm", () => ({
  limits: vi.fn(async () => ({ maxRecommendedBytes: 32 << 20 })),
  parseTrace: vi.fn(async () => ({ collection: {}, collection_id: "sample" })),
}));
vi.mock("./provider", () => ({ createInMemoryProvider: vi.fn(() => ({ kind: "sample" })) }));
vi.mock("../../web/src/App", () => ({ App: ({ setup }: { setup: { mode: string; status: string; selectedSample: string } }) => <main>sample viewer ready<span>{setup.mode}</span><span>{setup.status}</span><span>{setup.selectedSample}</span></main> }));

import { BrowserApp } from "./App";
import { parseTrace } from "./wasm";

describe("browser startup", () => {
  afterEach(() => { vi.unstubAllGlobals(); history.replaceState(null, "", "/"); });

  it("opens the bundled checkout cohort without an initial click", async () => {
    const fetch = vi.fn(async (_input: RequestInfo | URL) => new Response(new Uint8Array([1, 2, 3]), { status: 200 }));
    vi.stubGlobal("fetch", fetch);
    render(<BrowserApp />);

    expect(await screen.findByText("sample viewer ready")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(String(fetch.mock.calls[0][0])).toContain("checkout-cohort");
    expect(screen.getByText("browser")).toBeInTheDocument();
    expect(screen.getByText(/checkout-cohort\.ndjson is open/)).toBeInTheDocument();
    expect(screen.getByText("checkout-cohort.ndjson")).toBeInTheDocument();
    expect(screen.queryByRole("contentinfo")).not.toBeInTheDocument();
  });

  it("never paints the landing page while the bundled viewer initializes", async () => {
    let finish: ((value: Awaited<ReturnType<typeof parseTrace>>) => void) | undefined;
    vi.mocked(parseTrace).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
    vi.stubGlobal("fetch", vi.fn(async () => new Response(new Uint8Array([1, 2, 3]), { status: 200 })));
    render(<BrowserApp />);

    expect(screen.getByRole("status", { name: "Loading RLViz" })).toBeInTheDocument();
    expect(screen.queryByText("Inspect agent rollouts locally.")).not.toBeInTheDocument();
    finish?.({ collection: {}, collection_id: "sample" } as Awaited<ReturnType<typeof parseTrace>>);
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
    expect(screen.getByRole("button", { name: "verify and open" })).toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalled();
  });
});
