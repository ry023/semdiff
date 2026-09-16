import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Bootstrap, DiffItem, FragmentView } from "./types";
import { App, expandContext } from "./main";

const line = (id: string) => {
  const element = document.createElement("span");
  element.id = id;
  element.className = "context-hidden";
  element.hidden = true;
  return element;
};

const expandButton = (direction: "up" | "down") => {
  const button = document.createElement("button");
  button.className = "expand-lines";
  button.dataset.direction = direction;
  return button;
};

function bootstrap(): Bootstrap {
  const fragment: FragmentView & { header: DiffItem[] } = {
    id: "F1",
    path: "frontend/src/App.tsx",
    description: "Render the application shell.",
    review_level: "careful",
    range_label: "L10-L20",
    directory: "frontend/src/",
    name: "App.tsx",
    status: "updated",
    additions: 1,
    deletions: 0,
    diffstat: ["added", "neutral"],
    header: [
      {
        kind: "line",
        text: "diff --git a/frontend/src/App.tsx b/frontend/src/App.tsx",
        class: "meta",
      },
    ],
    hunk: [
      {
        kind: "line",
        text: `+${"x".repeat(20_050)}`,
        class: "add",
        new_number: "10",
      },
    ],
    upper_context: null,
    lower_context: null,
  };
  return {
    page: {
      base_sha: "base",
      head_sha: "head",
      groups: [
        {
          id: "group",
          title: "Group title",
          summary: "Group summary",
          importance: "core",
          anchor_id: "group-0",
          categories: [
            {
              name: "logic",
              icon: "logic",
              standard: true,
              files: [
                {
                  path: fragment.path,
                  anchor_id: "file-0-0",
                  directory: fragment.directory,
                  name: fragment.name,
                  status: fragment.status,
                  additions: fragment.additions,
                  deletions: fragment.deletions,
                  diffstat: fragment.diffstat,
                  fragments: [fragment],
                  review_level: "careful",
                },
              ],
              added: 0,
              updated: 1,
              deleted: 0,
            },
          ],
          steps: [
            {
              id: "step",
              title: "Review the shell",
              summary: "Start with the UI.",
              anchor_id: "step-0-0",
              number: 1,
              fragment_ids: [fragment.id],
            },
          ],
          fragment_count: 1,
        },
      ],
      sidebar_directories: null,
      sidebar_files: null,
      fragment_count: 1,
      file_count: 1,
    },
    capabilities: { questions: "disabled" },
  };
}

describe("expandContext", () => {
  it("moves a downward control after the last revealed line", () => {
    const container = document.createElement("span");
    const button = expandButton("down");
    const lines = Array.from({ length: 12 }, (_, index) =>
      line(`down-${index}`),
    );
    container.append(button, ...lines);

    expandContext(button, container);

    expect(button.previousElementSibling).toBe(lines[9]);
    expect(button.nextElementSibling).toBe(lines[10]);
    expect(button).toHaveTextContent("Show 2 lines below");
    expect(lines.slice(0, 10).every((item) => !item.hidden)).toBe(true);
    expect(lines.slice(10).every((item) => item.hidden)).toBe(true);

    expandContext(button, container);
    expect(container.querySelector(".expand-lines")).toBeNull();
  });

  it("moves an upward control before the first revealed line", () => {
    const container = document.createElement("span");
    const button = expandButton("up");
    const lines = Array.from({ length: 12 }, (_, index) => line(`up-${index}`));
    container.append(...lines, button);

    expandContext(button, container);

    expect(button.previousElementSibling).toBe(lines[1]);
    expect(button.nextElementSibling).toBe(lines[2]);
    expect(button).toHaveTextContent("Show 2 lines above");
    expect(lines.slice(0, 2).every((item) => item.hidden)).toBe(true);
    expect(lines.slice(2).every((item) => !item.hidden)).toBe(true);
  });
});

describe("viewer regressions", () => {
  it("renders guided and sidebar category markers without fragment internals", () => {
    const { container } = render(<App bootstrap={bootstrap()} />);

    expect(
      container.querySelector(".guided-file .category-badge"),
    ).toHaveTextContent("logic");
    expect(
      container.querySelector(".nav-group-file .category-icon"),
    ).toHaveAttribute("title", "logic");
    expect(screen.getAllByText("App.tsx").length).toBeGreaterThan(0);
    expect(screen.queryByText("F1")).not.toBeInTheDocument();
    expect(screen.queryByText("L10-L20")).not.toBeInTheDocument();
    expect(screen.queryByText(/diff --git/)).not.toBeInTheDocument();
  });

  it("limits giant lines and only mounts the selected diff layout", () => {
    const { container } = render(<App bootstrap={bootstrap()} />);

    expect(container.querySelectorAll(".unified-cell")).toHaveLength(2);
    expect(container.querySelectorAll(".split-cell")).toHaveLength(0);
    expect(container.querySelector(".line-code")).toHaveTextContent(
      "[50 characters omitted]",
    );

    fireEvent.click(screen.getByRole("button", { name: "Split" }));

    expect(container.querySelectorAll(".unified-cell")).toHaveLength(0);
    expect(container.querySelectorAll(".split-cell")).toHaveLength(4);
  });

  it("switches between guided review and categorized files", () => {
    const { container } = render(<App bootstrap={bootstrap()} />);

    expect(container.querySelector(".guided-group")).toBeInTheDocument();
    expect(container.querySelector(".files-group")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Files" }));

    expect(container.querySelector(".guided-group")).not.toBeInTheDocument();
    expect(container.querySelector(".files-group")).toBeInTheDocument();
    expect(container.querySelector(".category > summary")).toHaveTextContent(
      "logic",
    );
  });

  it("marks a guided fragment reviewed and collapses its diff", () => {
    const { container } = render(<App bootstrap={bootstrap()} />);
    const fragment =
      container.querySelector<HTMLDetailsElement>(".guided-file");
    const toggle = container.querySelector<HTMLButtonElement>(
      ".guided-file .file-review-toggle",
    );

    expect(fragment).toHaveAttribute("open");
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(toggle!);

    expect(fragment).not.toHaveAttribute("open");
    expect(fragment).toHaveClass("is-reviewed");
    expect(toggle).toHaveAttribute("aria-pressed", "true");
  });

  it("does not perform synchronous work from a window scroll handler", () => {
    const addEventListener = vi.spyOn(window, "addEventListener");

    render(<App bootstrap={bootstrap()} />);

    expect(
      addEventListener.mock.calls.some(([event]) => event === "scroll"),
    ).toBe(false);
    expect(
      addEventListener.mock.calls.some(([event]) => event === "resize"),
    ).toBe(true);
  });
});
