import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AnalysisPanel } from "./AnalysisPanel";
import type { AnalysisResponse } from "./types";

const result: AnalysisResponse = {
  cached: true, analyzed_at: "2026-07-16T12:30:00Z",
  analysis: {
    api_version: "rolloutviz.dev/analyzer/v1alpha1",
    provenance: { name: "builtin.loop-retry", version: "0.1.0", digest: "sha256:analyzer", input_digest: "sha256:input" },
    findings: [{ id: "finding", trajectory_id: "trajectory", event_ids: ["event-a", "event-b", "event-c"], kind: "retry", severity: "warning", title: "Repeated identical action", summary: "The same action repeated three times." }],
    signals: [{ id: "signal", trajectory_id: "trajectory", event_id: "event-c", name: "analyzer.loop_retry.detected", value: true }],
  },
};

describe("analysis panel", () => {
  it("shows provenance, cache state, findings, and derived signals", () => {
    render(<AnalysisPanel analysis={result} loading={false} error="" onRetry={() => {}} onJump={() => {}} />);
    expect(screen.getByText("cache hit")).toBeInTheDocument();
    expect(screen.getByText("builtin.loop-retry")).toBeInTheDocument();
    expect(screen.getByText("Repeated identical action")).toBeInTheDocument();
    expect(screen.getByText("loop_retry.detected")).toBeInTheDocument();
  });

  it("jumps from a finding or any referenced event", () => {
    const jump = vi.fn();
    render(<AnalysisPanel analysis={result} loading={false} error="" onRetry={() => {}} onJump={jump} />);
    fireEvent.click(screen.getByRole("button", { name: /Repeated identical action/ }));
    expect(jump).toHaveBeenLastCalledWith("event-a");
    fireEvent.click(screen.getByRole("button", { name: "#3" }));
    expect(jump).toHaveBeenLastCalledWith("event-c");
  });

  it("has resilient loading, empty, and retryable error states", () => {
    const retry = vi.fn();
    const { rerender } = render(<AnalysisPanel analysis={null} loading error="" onRetry={retry} onJump={() => {}} />);
    expect(screen.getByText("Analyzing trajectory…")).toBeInTheDocument();
    rerender(<AnalysisPanel analysis={null} loading={false} error="index unavailable" onRetry={retry} onJump={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(retry).toHaveBeenCalledOnce();
    rerender(<AnalysisPanel analysis={{ ...result, analysis: { ...result.analysis, findings: [], signals: [] } }} loading={false} error="" onRetry={retry} onJump={() => {}} />);
    expect(screen.getByText("No loops or retries")).toBeInTheDocument();
  });
});
