import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Inspector } from "./Inspector";
import type { PresentationConfig, TrajectoryEvent } from "./types";

const event: TrajectoryEvent = {
  id: "event-1",
  sequence: 1,
  kind: "tool",
  title: "read_file",
  input: { path: "README.md" },
  output: { bytes: 42 },
  content: "tool exchange",
  metadata: { attempt: 1 },
  source: { path: "trace.jsonl", line: 2 },
};

function renderInspector(presentation: PresentationConfig | undefined, raw = false) {
  return render(<Inspector event={event} raw={raw} presentation={presentation} analysis={null} analysisLoading={false} analysisError="" onRetryAnalysis={vi.fn()} onJump={vi.fn()} artifacts={[]} sourceId="source" trajectoryId="trajectory" selectedArtifactId="" onSelectArtifact={vi.fn()} />);
}

describe("declarative inspector layout", () => {
  it("uses the exact configured order and hides omitted sections", () => {
    const presentation: PresentationConfig = { api_version: "rlviz.dev/v1alpha1", inspector: { sections: ["analysis", "source", "properties"] } };
    const { container } = renderInspector(presentation);
    expect([...container.querySelectorAll(".inspector-scroll > section")].map((node) => node.getAttribute("aria-label") ?? node.querySelector("h4")?.textContent)).toEqual(["Analyzer findings", "Source", "Properties"]);
    expect(screen.getByRole("region", { name: "Analyzer findings" })).toBeInTheDocument();
    expect(screen.getByText("Source", { selector: "h4" })).toBeInTheDocument();
    expect(screen.queryByText("Input", { selector: "h4" })).not.toBeInTheDocument();
  });

  it("keeps the core-owned raw record available regardless of configured sections", () => {
    const presentation: PresentationConfig = { api_version: "rlviz.dev/v1alpha1", inspector: { sections: ["analysis"] } };
    const { container } = renderInspector(presentation, true);
    expect(screen.getByText("Raw normalized record")).toBeInTheDocument();
    expect(screen.getByText(/README.md/, { selector: ".raw-json" })).toBeInTheDocument();
    expect(container.querySelectorAll(".inspector-scroll > section")).toHaveLength(1);
  });
});
