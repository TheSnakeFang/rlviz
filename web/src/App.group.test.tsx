import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import type { ViewerProvider } from "./provider";
import type { BrowseResponse, ComparisonResponse, RolloutQueryResponse } from "./types";
import { emptyWorkspace, laneId, serializeWorkspace } from "./workspace";

const browse: BrowseResponse = {
  sources: [{ id: "source-1", path: "/tmp/demo.ndjson", index_state: "complete" }], count: 2,
  trajectories: [
    { source_id: "source-1", source_name: "demo.ndjson", case_name: "demo", trajectory: { id: "candidate", group_id: "group", status: "failed" }, metrics: { trajectory: { id: "candidate", group_id: "group" }, event_count: 2, error_count: 1, pass: false, reward: -1 } },
    { source_id: "source-1", source_name: "demo.ndjson", case_name: "demo", trajectory: { id: "reference", group_id: "group", status: "completed" }, metrics: { trajectory: { id: "reference", group_id: "group" }, event_count: 2, error_count: 0, pass: true, reward: 1 } },
  ],
};
const trajectoryPayload = (id: string) => ({ trajectory: { id, group_id: "group", status: id === "candidate" ? "failed" : "completed" }, events: [{ id: `${id}-start`, sequence: 0, kind: "tool", alignment_key: "stage:work", output: { ok: true } }, { id: `${id}-end`, sequence: 10, kind: id === "candidate" ? "error" : "grader", alignment_key: "stage:outcome", output: { verdict: id === "candidate" ? "fail" : "pass" } }], page: { count: 2, total: 2, limit: 200, has_more: false } });
const comparison: ComparisonResponse = {
  left: { trajectory: { id: "candidate" }, events: trajectoryPayload("candidate").events },
  right: { trajectory: { id: "reference" }, events: trajectoryPayload("reference").events },
  alignment: { common_behavioral_prefix: 0, first_meaningful_divergence: 0, steps: [] },
  differences: { event_count: { left: 2, right: 2, delta: 0 }, status: { changed: true }, termination: { changed: false }, reward: { changed: true } },
};

describe("Browse Read Compare flow", () => {
  afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState({}, "", "/"); });

  it("groups trials by task and variant with source-backed aggregate metrics", async () => {
    const rows: BrowseResponse = {
      sources: [{ id: "source-1", path: "/tmp/eval.ndjson", index_state: "complete" }], count: 3,
      trajectories: [
        { source_id: "source-1", source_name: "eval.ndjson", run_name: "Checkout evaluation", case_name: "Saved card checkout", group_name: "Deliberate", trajectory: { id: "pass-1", group_id: "deliberate", status: "completed" }, metrics: { event_count: 10, pass: true, signals: { token_count: 120, cost_usd: 0.012 } } },
        { source_id: "source-1", source_name: "eval.ndjson", run_name: "Checkout evaluation", case_name: "Saved card checkout", group_name: "Deliberate", trajectory: { id: "fail-1", group_id: "deliberate", status: "failed" }, metrics: { event_count: 14, pass: false, signals: { token_count: 180, cost_usd: 0.018 } } },
        { source_id: "source-1", source_name: "eval.ndjson", run_name: "Checkout evaluation", case_name: "Saved card checkout", group_name: "Direct", trajectory: { id: "infra-1", group_id: "direct", status: "failed", termination: "infrastructure_timeout" }, metrics: { event_count: 4, signals: { token_count: 40, cost_usd: 0.004, failure_class: "infrastructure" } } },
      ],
    };
    const provider: ViewerProvider = {
      async loadInitial() { return { trajectory: { ...trajectoryPayload("pass-1").trajectory, events: trajectoryPayload("pass-1").events }, isSample: false, presentation: { api_version: "rlviz.dev/v1alpha1", fields: { "signal:cost_usd": { label: "cost" } }, scalars: { "signal:cost_usd": { format: "number", precision: 3, unit: "USD" } }, group: { columns: ["signal:cost_usd"] } } }; },
      async loadBrowse() { return rows; },
      async loadTrajectory() { return { trajectory: { ...trajectoryPayload("pass-1").trajectory, events: trajectoryPayload("pass-1").events }, isSample: false }; },
      async loadAnalysis() { return { analysis: { api_version: "v1", provenance: { name: "test", version: "1", digest: "x", input_digest: "y" } }, cached: false, analyzed_at: "now" }; },
      async loadComparison() { return comparison; },
      async loadArtifactContent() { throw new Error("unused"); },
    };
    render(<App provider={provider} />);
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(3));
    fireEvent.click(screen.getByRole("button", { name: "trials" }));
    expect(screen.getByRole("group", { name: "Saved card checkout" })).toHaveTextContent("Checkout evaluation");
    expect(screen.getByRole("group", { name: "Deliberate" })).toHaveTextContent("1/2 pass");
    expect(screen.getByRole("group", { name: "Deliberate" })).toHaveTextContent("avg 12 steps");
    expect(screen.getByRole("group", { name: "Deliberate" })).toHaveTextContent("avg 150 tokens");
    expect(screen.getByRole("group", { name: "Deliberate" })).toHaveTextContent("cost avg 0.015 USD");
    expect(screen.getByRole("group", { name: "Direct" })).toHaveTextContent("1 infra");
    expect(screen.getByRole("group", { name: "Direct" })).toHaveTextContent("1 timeout");
  });

  it("queries and paginates the indexed rollout cohort from the collection rail", async () => {
    const indexedRow = (id: string, reward: number) => ({
      source_id: "source-1", source_name: "eval.ndjson", run_id: "run-1", run_name: "Evaluation",
      case_id: "case-1", case_name: "Checkout", group_id: "group-1", group_name: "Candidate", checkpoint: "ckpt-7",
      summary: { trajectory: { id, group_id: "group-1", status: "completed" }, reward, event_count: 3, tool_call_count: 1, cost_usd: 0.01, signals: { cost_usd: 0.01 } },
    });
    const queryRollouts = vi.fn(async (params) => params.offset === 1 ? {
      rollouts: [indexedRow("indexed-2", 0.8)],
      aggregates: { count: 2, success: 2, failure: 0, unknown: 0, total_cost_usd: 0.02 },
      page: { count: 1, total: 2, limit: 1, offset: 1, has_more: false },
    } : {
      rollouts: [indexedRow("indexed-1", 0.9)],
      aggregates: { count: 2, success: 2, failure: 0, unknown: 0, total_cost_usd: 0.02 },
      page: { count: 1, total: 2, limit: 1, offset: 0, next_offset: 1, has_more: true },
    });
    const provider: ViewerProvider = {
      async loadInitial() { return { trajectory: { ...trajectoryPayload("candidate").trajectory, events: trajectoryPayload("candidate").events }, isSample: false }; },
      async loadBrowse() { return browse; },
      queryRollouts,
      async loadTrajectory() { return { trajectory: { ...trajectoryPayload("candidate").trajectory, events: trajectoryPayload("candidate").events }, isSample: false }; },
      async loadAnalysis() { return { analysis: { api_version: "v1", provenance: { name: "test", version: "1", digest: "x", input_digest: "y" } }, cached: false, analyzed_at: "now" }; },
      async loadComparison() { return comparison; },
      async loadArtifactContent() { throw new Error("unused"); },
    };
    render(<App provider={provider} />);
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
    const filter = screen.getByLabelText("Filter");
    fireEvent.change(filter, { target: { value: "checkpoint:ckpt-7 reward>=0.5 tool:browser" } });
    fireEvent.keyDown(filter, { key: "Enter" });
    await waitFor(() => expect(queryRollouts).toHaveBeenCalledWith(expect.objectContaining({ checkpoint: "ckpt-7", reward_min: 0.5, tool: "browser", offset: 0 }), expect.any(AbortSignal)));
    expect(await screen.findByRole("status", { name: "" })).toHaveTextContent("2 matches · $0.0200");
    expect(screen.getAllByRole("option")).toHaveLength(1);
    expect(screen.getByRole("option")).toHaveTextContent("indexed-1");
    fireEvent.click(screen.getByRole("button", { name: "load more" }));
    await waitFor(() => expect(queryRollouts).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 1 }), expect.any(AbortSignal)));
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
    expect(screen.getByRole("option", { name: /indexed-2/ })).toBeInTheDocument();
  });

  it("does not restore a stale indexed page after the filter changes", async () => {
    const first: RolloutQueryResponse = {
      rollouts: [{
        source_id: "source-1", source_name: "eval.ndjson", run_id: "run-1", case_id: "case-1", group_id: "group-1",
        summary: { trajectory: { id: "indexed-1", group_id: "group-1" }, event_count: 1 },
      }],
      aggregates: { count: 2, success: 0, failure: 0, unknown: 2 },
      page: { count: 1, total: 2, limit: 1, offset: 0, next_offset: 1, has_more: true },
    };
    let resolveNext!: (value: RolloutQueryResponse) => void;
    const next = new Promise<RolloutQueryResponse>((resolve) => { resolveNext = resolve; });
    const queryRollouts = vi.fn(async (params) => params.offset === 1 ? next : first);
    const provider: ViewerProvider = {
      async loadInitial() { return { trajectory: { ...trajectoryPayload("candidate").trajectory, events: trajectoryPayload("candidate").events }, isSample: false }; },
      async loadBrowse() { return browse; },
      queryRollouts,
      async loadTrajectory() { return { trajectory: { ...trajectoryPayload("candidate").trajectory, events: trajectoryPayload("candidate").events }, isSample: false }; },
      async loadAnalysis() { return { analysis: { api_version: "v1", provenance: { name: "test", version: "1", digest: "x", input_digest: "y" } }, cached: false, analyzed_at: "now" }; },
      async loadComparison() { return comparison; },
      async loadArtifactContent() { throw new Error("unused"); },
    };
    render(<App provider={provider} />);
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
    const filter = screen.getByLabelText("Filter");
    fireEvent.change(filter, { target: { value: "status:failed" } });
    fireEvent.keyDown(filter, { key: "Enter" });
    await waitFor(() => expect(screen.getByRole("option")).toHaveTextContent("indexed-1"));
    fireEvent.click(screen.getByRole("button", { name: "load more" }));
    await waitFor(() => expect(queryRollouts).toHaveBeenCalledTimes(2));
    fireEvent.change(filter, { target: { value: "candidate" } });
    expect(screen.getByRole("option")).toHaveTextContent("candidate");
    resolveNext({
      rollouts: [{
        source_id: "source-1", source_name: "eval.ndjson", run_id: "run-1", case_id: "case-1", group_id: "group-1",
        summary: { trajectory: { id: "stale-indexed-2", group_id: "group-1" }, event_count: 1 },
      }],
      aggregates: first.aggregates,
      page: { count: 1, total: 2, limit: 1, offset: 1, has_more: false },
    });
    await waitFor(() => expect(screen.queryByText("stale-indexed-2")).not.toBeInTheDocument());
    expect(screen.getByRole("option")).toHaveTextContent("candidate");
  });

  it("loads the daemon collection and composes a two-lane reference arrangement", async () => {
    window.history.replaceState({}, "", "/#token=secret");
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(browse));
      if (url.includes("/indexed/compare")) return new Response(JSON.stringify(comparison));
      if (url.includes("/indexed/analysis")) return new Response(JSON.stringify({ analysis: { api_version: "v1", provenance: { name: "test", version: "1", digest: "x", input_digest: "y" }, findings: [], signals: [] }, cached: false, analyzed_at: "now" }));
      if (url.includes("/indexed/trajectory")) {
        const id = new URL(url, "http://local").searchParams.get("trajectory_id") ?? "candidate";
        return new Response(JSON.stringify(trajectoryPayload(id)));
      }
      throw new Error(`unexpected request ${url}`);
    }));
    render(<App />);
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
		expect(screen.getAllByRole("option")[0]).toHaveTextContent("candidate");
		expect(screen.getByRole("option", { selected: true })).toHaveTextContent("candidate");
		expect(screen.getByText(/2 trajectories/)).toBeInTheDocument();
		fireEvent.keyDown(window, { key: "j" });
		expect(screen.getByRole("option", { selected: true })).toHaveTextContent("reference");
		fireEvent.keyDown(window, { key: "k" });
    fireEvent.keyDown(window, { key: "Enter" });
    expect(await screen.findByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-trajectory", "candidate");
    fireEvent.keyDown(window, { key: "Tab" });
    fireEvent.keyDown(window, { key: "Tab" });
    fireEvent.keyDown(window, { key: "j" });
    fireEvent.keyDown(window, { key: "a" });
    await waitFor(() => expect(screen.getAllByRole("main", { name: "Read trajectory" })).toHaveLength(2));
    expect(screen.getAllByRole("main", { name: "Read trajectory" }).map((lane) => lane.getAttribute("data-trajectory"))).toEqual(["candidate", "reference"]);
    fireEvent.keyDown(window, { key: "Tab", shiftKey: true });
    fireEvent.keyDown(window, { key: "Tab", shiftKey: true });
    fireEvent.keyDown(window, { key: "+", code: "Equal", shiftKey: true });
    expect(screen.getAllByRole("main", { name: "Read trajectory" })[1]).toHaveAttribute("data-axis-end", "7.0000");
    expect(screen.getAllByRole("main", { name: "Read trajectory" })[0]).toHaveAttribute("data-axis-end", "10.0000");
    fireEvent.keyDown(window, { key: "A", shiftKey: true });
    expect(screen.getByTestId("reference-name")).toHaveTextContent("reference");
    fireEvent.keyDown(window, { key: "V", shiftKey: true });
    expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-direction", "columns");
    fireEvent.keyDown(window, { key: "x" });
    expect(screen.getAllByRole("main", { name: "Read trajectory" })).toHaveLength(1);
    expect(screen.getByTestId("reference-name")).toHaveTextContent("none");
  });

	it("keeps the newest rollout sweep when an older load resolves last", async () => {
		const rows = ["candidate", "reference", "third"].map((id) => ({
			...browse.trajectories[0],
			trajectory: { ...browse.trajectories[0].trajectory, id },
			metrics: { ...browse.trajectories[0].metrics, trajectory: { id } },
		}));
		const collection = { ...browse, count: rows.length, trajectories: rows };
		const pending = new Map<string, { resolve: (value: Awaited<ReturnType<ViewerProvider["loadTrajectory"]>>) => void; signal?: AbortSignal }>();
		const provider: ViewerProvider = {
			async loadInitial() { return { trajectory: { ...trajectoryPayload("candidate").trajectory, events: trajectoryPayload("candidate").events }, isSample: false }; },
			async loadBrowse() { return collection; },
			loadTrajectory(_sourceId, trajectoryId, signal) {
				if (trajectoryId === "candidate") return Promise.resolve({ trajectory: { ...trajectoryPayload("candidate").trajectory, events: trajectoryPayload("candidate").events }, isSample: false });
				return new Promise((resolve) => pending.set(trajectoryId, { resolve, signal }));
			},
			async loadAnalysis() { return { analysis: { api_version: "v1", provenance: { name: "test", version: "1", digest: "x", input_digest: "y" } }, cached: false, analyzed_at: "now" }; },
			async loadComparison() { return comparison; },
			async loadArtifactContent() { throw new Error("unused"); },
		};
		render(<App provider={provider} />);
		await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(3));
		fireEvent.keyDown(window, { key: "Enter" });
		await screen.findByRole("main", { name: "Read trajectory" });
		fireEvent.keyDown(window, { key: "n" });
		fireEvent.keyDown(window, { key: "p" });
		await waitFor(() => expect(pending.size).toBe(2));
		expect(pending.get("reference")?.signal?.aborted).toBe(true);
		pending.get("third")!.resolve({ trajectory: { ...trajectoryPayload("third").trajectory, events: trajectoryPayload("third").events }, isSample: false });
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-trajectory", "third"));
		pending.get("reference")!.resolve({ trajectory: { ...trajectoryPayload("reference").trajectory, events: trajectoryPayload("reference").events }, isSample: false });
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-trajectory", "third"));
		expect(document.querySelector(".instrument-shell")?.getAttribute("data-active-zone")).toBe(laneId("source-1", "third"));
	});

	it("does not apply late analysis to another rollout", async () => {
		let resolveCandidateAnalysis!: (value: Response) => void;
		vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(browse));
			if (url === "/api/v1/trajectory") return new Response(JSON.stringify(trajectoryPayload("candidate")));
			if (url.includes("/indexed/trajectory")) {
				const id = new URL(url, "http://local").searchParams.get("trajectory_id") ?? "candidate";
				return new Response(JSON.stringify(trajectoryPayload(id)));
			}
			if (url.includes("/indexed/analysis")) {
				const id = new URL(url, "http://local").searchParams.get("trajectory_id");
				if (id === "candidate") return new Promise<Response>((resolve) => { resolveCandidateAnalysis = resolve; });
				return new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [], signals: [] } }));
			}
			throw new Error(`unexpected request ${url}`);
		}));
		render(<App />);
		await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
		fireEvent.keyDown(window, { key: "Enter" });
		expect(await screen.findByRole("main", { name: "Read trajectory" })).toHaveTextContent("candidate");
		fireEvent.keyDown(window, { key: "Escape" });
		fireEvent.keyDown(window, { key: "j" });
		fireEvent.keyDown(window, { key: "Enter" });
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("reference"));
		resolveCandidateAnalysis(new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [{ event_ids: ["candidate-start"] }], signals: [] } })));
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("reference"));
	});

	it("does not move a deliberate event-zero selection when initial analysis arrives", async () => {
		const plain = { trajectory: { id: "plain" }, events: [
			{ id: "plain-start", sequence: 0, kind: "message", title: "start" },
			{ id: "plain-finding", sequence: 1, kind: "tool", title: "finding" },
		] };
		const collection: BrowseResponse = { sources: [{ id: "source" }], count: 1, trajectories: [{ source_id: "source", source_name: "plain", trajectory: { id: "plain" }, metrics: { event_count: 2 } }] };
		let resolveAnalysis!: (value: Response) => void;
		vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(collection));
			if (url.includes("/indexed/analysis")) return new Promise<Response>((resolve) => { resolveAnalysis = resolve; });
			return new Response(JSON.stringify(plain));
		}));
		render(<App />);
		await screen.findByRole("option");
		fireEvent.keyDown(window, { key: "Enter" });
		await screen.findByRole("main", { name: "Read trajectory" });
		fireEvent.click(screen.getByRole("button", { name: /start/ }));
		resolveAnalysis(new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [{ event_ids: ["plain-finding"] }], signals: [] } })));
		await waitFor(() => expect(screen.getByText("start", { selector: ".moment.selected b" })).toBeInTheDocument());
	});

	it("walks the filtered attention queue with n and p", async () => {
		const collection: BrowseResponse = {
			...browse,
			count: 3,
			trajectories: [
				{ ...browse.trajectories[0], case_name: "keep" },
				{ ...browse.trajectories[1], case_name: "keep" },
				{ source_id: "source-1", source_name: "demo.ndjson", case_name: "drop", trajectory: { id: "excluded" }, metrics: { event_count: 1, error_count: 0 } },
			],
		};
		vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(collection));
			if (url.includes("/indexed/analysis")) return new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [], signals: [] } }));
			if (url.includes("/indexed/trajectory")) {
				const id = new URL(url, "http://local").searchParams.get("trajectory_id") ?? "candidate";
				return new Response(JSON.stringify(trajectoryPayload(id)));
			}
			return new Response(JSON.stringify(trajectoryPayload("candidate")));
		}));
		render(<App />);
		await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(3));
		fireEvent.change(screen.getByLabelText("Filter"), { target: { value: "keep" } });
		expect(screen.getAllByRole("option")).toHaveLength(2);
		fireEvent.keyDown(window, { key: "Enter" });
		await screen.findByRole("main", { name: "Read trajectory" });
		expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("candidate");
		fireEvent.keyDown(window, { key: "+" });
		fireEvent.keyDown(window, { key: "Enter" });
		const axisStart = screen.getByRole("main", { name: "Read trajectory" }).getAttribute("data-axis-start");
		fireEvent.keyDown(window, { key: "n" });
		await waitFor(() => {
			expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("reference");
			expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-axis-start", axisStart);
			expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveAttribute("data-depth", "2");
		});
		fireEvent.keyDown(window, { key: "p" });
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).toHaveTextContent("candidate"));
	});

	it("sweeps the active context lane without replacing either focus lane", async () => {
		const collection: BrowseResponse = {
			...browse, count: 4, trajectories: [
				browse.trajectories[0], browse.trajectories[1],
				{ ...browse.trajectories[0], trajectory: { id: "third" }, metrics: { event_count: 2 } },
				{ ...browse.trajectories[0], trajectory: { id: "fourth" }, metrics: { event_count: 2 } },
			],
		};
		vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(collection));
			if (url.includes("/indexed/analysis")) return new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [], signals: [] } }));
			if (url.includes("/indexed/trajectory")) return new Response(JSON.stringify(trajectoryPayload(new URL(url, "http://local").searchParams.get("trajectory_id") ?? "candidate")));
			return new Response(JSON.stringify(trajectoryPayload("candidate")));
		}));
		render(<App />);
		await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(4));
		fireEvent.keyDown(window, { key: "Enter" });
		await screen.findByRole("main", { name: "Read trajectory" });
		// `a` piles lanes in while focus stays in the collection.
		fireEvent.keyDown(window, { key: "Tab" }); fireEvent.keyDown(window, { key: "Tab" }); fireEvent.keyDown(window, { key: "j" }); fireEvent.keyDown(window, { key: "a" });
		await waitFor(() => expect(screen.getAllByRole("main", { name: "Read trajectory" })).toHaveLength(2));
		expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-active-zone", "rail");
		// add keeps the collection focused, so the next add needs no Tab round-trip
		fireEvent.keyDown(window, { key: "j" }); fireEvent.keyDown(window, { key: "a" });
		await screen.findByRole("main", { name: "Context lane third" });
		expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-active-zone", "rail");
		fireEvent.keyDown(window, { key: "Tab", shiftKey: true });
		fireEvent.keyDown(window, { key: "Tab", shiftKey: true });
		fireEvent.keyDown(window, { key: "n" });
		await screen.findByRole("main", { name: "Context lane fourth" });
		expect(screen.getAllByRole("main", { name: "Read trajectory" }).map((lane) => lane.getAttribute("data-trajectory"))).toEqual(["candidate", "reference"]);
	});

	it("keeps the active zone visible when the rail is collapsed and the last lane closes", async () => {
		vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
			const url = String(input);
			if (url === "/api/v1/indexed/browse") return new Response(JSON.stringify(browse));
			if (url.includes("/indexed/analysis")) return new Response(JSON.stringify({ analysis: { provenance: { name: "test" }, findings: [], signals: [] } }));
			if (url.includes("/indexed/trajectory")) return new Response(JSON.stringify(trajectoryPayload(new URL(url, "http://local").searchParams.get("trajectory_id") ?? "candidate")));
			return new Response(JSON.stringify(trajectoryPayload("candidate")));
		}));
		render(<App />); await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(2));
		fireEvent.keyDown(window, { key: "Enter" }); await screen.findByRole("main", { name: "Read trajectory" });
		fireEvent.keyDown(window, { key: "t" }); fireEvent.keyDown(window, { key: "Tab" });
		expect(screen.queryByRole("main", { name: "Browse trajectories" })).not.toBeInTheDocument();
		expect(document.querySelector(".instrument-shell")).not.toHaveAttribute("data-active-zone", "rail");
		// Detail is now independently closeable. Closing it returns focus to the
		// rollout; closing the final rollout then restores the collection.
		fireEvent.keyDown(window, { key: "x" });
		expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-active-zone", "source-1:candidate");
		fireEvent.keyDown(window, { key: "x" });
		expect(await screen.findByRole("main", { name: "Browse trajectories" })).toBeInTheDocument();
		expect(document.querySelector(".instrument-shell")).toHaveAttribute("data-active-zone", "rail");
	});

	it("reloads a lane restored from the jumplist after it was closed while loading", async () => {
		const state = emptyWorkspace(); const id = laneId("source-1", "candidate");
		state.lanes = [{ id, sourceId: "source-1", trajectoryId: "candidate", band: "focus", selected: 0, depth: 1, fidelity: 3, axis: { start: 0, end: 1 }, descentStack: [] }]; state.active = id;
		window.history.replaceState({ rlvizWorkspace: state }, "", `/?workspace=${encodeURIComponent(serializeWorkspace(state))}`);
		const loads: Array<(value: Awaited<ReturnType<ViewerProvider["loadTrajectory"]>>) => void> = [];
		const provider: ViewerProvider = {
			async loadInitial() { return { trajectory: trajectoryPayload("candidate").trajectory as never, isSample: false }; },
			async loadBrowse() { return browse; },
			loadTrajectory() { return new Promise((resolve) => loads.push(resolve)); },
			async loadAnalysis() { return { analysis: { api_version: "v1", provenance: { name: "test", version: "1", digest: "x", input_digest: "y" } }, cached: false, analyzed_at: "now" }; },
			async loadComparison() { return comparison; },
			async loadArtifactContent() { throw new Error("unused"); },
		};
		render(<App provider={provider} />); await waitFor(() => expect(loads).toHaveLength(1));
		fireEvent.keyDown(window, { key: "x" }); fireEvent.keyDown(window, { key: "o", ctrlKey: true });
		await waitFor(() => expect(loads).toHaveLength(2));
		loads[1]({ trajectory: { ...trajectoryPayload("candidate").trajectory, events: trajectoryPayload("candidate").events }, isSample: false });
		await waitFor(() => expect(screen.getByRole("main", { name: "Read trajectory" })).not.toHaveTextContent("loading trajectory"));
	});
});
