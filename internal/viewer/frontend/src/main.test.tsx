import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Bootstrap, DiffItem, FragmentView } from "./types";
import { App, expandContext } from "./main";
import { detectLocale } from "./i18n";

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
  it("uses Japanese when it appears in browser language preferences", () => {
    expect(detectLocale(["fr-FR", "ja-JP", "en-US"])).toBe("ja");
    const originalLanguages = Object.getOwnPropertyDescriptor(
      navigator,
      "languages",
    );
    Object.defineProperty(navigator, "languages", {
      configurable: true,
      value: ["ja-JP", "en-US"],
    });
    try {
      const { container, unmount } = render(<App bootstrap={bootstrap()} />);
      expect(container.querySelector(".page-header h1")).toHaveTextContent(
        "Semantic Review",
      );
      expect(container.querySelector(".stats")).toHaveTextContent(
        "1 Group · 1 ファイル · 1 Fragment",
      );
      expect(
        screen.getByRole("button", { name: "Guided" }),
      ).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Files" })).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Unified" }),
      ).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Split" })).toBeInTheDocument();
      expect(
        container.querySelector(".guided-group > summary h2"),
      ).toHaveTextContent("Group 1. Group title");
      expect(
        container.querySelector(".review-step > summary h3"),
      ).toHaveTextContent("Step 1. Review the shell");
      expect(
        container.querySelector(".guided-group > summary .count"),
      ).toHaveTextContent("1 Step · 1 Fragment");
      expect(
        container.querySelector(".guided-group > summary .importance"),
      ).toHaveTextContent("Core");
      expect(
        container.querySelector(".guided-file .review-level"),
      ).toHaveAttribute("aria-label", "Careful");
      expect(
        container.querySelector(".guided-file .category-badge-label"),
      ).toHaveTextContent("ロジック");
      expect(document.documentElement.lang).toBe("ja");
      unmount();
    } finally {
      if (originalLanguages)
        Object.defineProperty(navigator, "languages", originalLanguages);
      else delete (navigator as { languages?: readonly string[] }).languages;
    }
  });

  it("renders the review controls in one page-wide header", () => {
    const { container } = render(<App bootstrap={bootstrap()} />);
    const header = container.querySelector(".viewer-shell > .page-header");

    expect(header).toHaveTextContent("Semantic Review");
    expect(header).toHaveTextContent("base → head");
    expect(header).toHaveTextContent("1 groups · 1 files · 1 fragments");
    expect(header).toContainElement(
      screen.getByRole("button", { name: "Guided" }),
    );
    expect(header).toContainElement(
      screen.getByRole("button", { name: "Unified" }),
    );
    expect(container.querySelector(".app-shell > .page-header")).toBeNull();
  });

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

  it("explains importance and review level on keyboard focus", async () => {
    const { container } = render(<App bootstrap={bootstrap()} />);

    const importance = container.querySelector<HTMLElement>(
      ".guided-group > summary .importance",
    )!;
    importance.focus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent(
      "Defines the PR's purpose or essential behavior.",
    );

    const level = container.querySelector<HTMLElement>(
      ".guided-file .review-level",
    )!;
    level.focus();
    await waitFor(() =>
      expect(screen.getByRole("tooltip")).toHaveTextContent(
        "Read this fragment closely.",
      ),
    );
  });

  it("keeps group and step summaries in their collapsible headers", () => {
    const data = bootstrap();
    data.page.groups![0].summary =
      "Why: First line.\n\n- What: First item.\n- So what: Second item.";
    data.page.groups![0].steps![0].summary =
      "First paragraph.\n\n- First item.\n- Second item.";
    const { container } = render(<App bootstrap={data} />);

    const group = container.querySelector(".guided-group")!;
    const step = container.querySelector(".review-step")!;
    fireEvent.click(group.querySelector(":scope > summary")!);
    expect(group).not.toHaveAttribute("open");
    const groupSummary = group.querySelector(
      ":scope > summary .summary-preview",
    )!;
    expect(groupSummary.querySelector("p")).toHaveTextContent(
      "Why: First line.",
    );
    expect(groupSummary.querySelectorAll("li")).toHaveLength(2);
    expect(step).not.toHaveAttribute("open");
    const stepSummary = step.querySelector(
      ":scope > summary .summary-preview",
    )!;
    expect(stepSummary.querySelector("p")).toHaveTextContent(
      "First paragraph.",
    );
    expect(stepSummary.querySelectorAll("li")).toHaveLength(2);
    expect(container.querySelector(".group-summary")).toBeNull();
    expect(container.querySelector(".step-summary")).toBeNull();
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

  it("switches between guided review and categorized files", async () => {
    const { container } = render(<App bootstrap={bootstrap()} />);

    expect(container.querySelector(".guided-group")).toBeInTheDocument();
    expect(container.querySelector(".files-group")).not.toBeInTheDocument();
    expect(
      container.querySelector(".guided-group > summary h2"),
    ).toHaveTextContent("Group 1. Group title");
    expect(
      container.querySelector(".sidebar .nav-group > summary"),
    ).toHaveTextContent("Group 1. Group title");
    expect(container.querySelector(".sidebar .nav-step")).toHaveTextContent(
      "Step 1. Review the shell",
    );
    expect(container.querySelector(".sidebar .nav-step")).not.toHaveAttribute(
      "open",
    );
    const mainStep = container.querySelector(".review-step");
    const sidebarStep = container.querySelector(".sidebar .nav-step");
    expect(mainStep!.querySelector("h3")).toHaveTextContent(
      "Step 1. Review the shell",
    );
    expect(mainStep).not.toHaveAttribute("open");
    fireEvent.click(mainStep!.querySelector("summary")!);
    expect(mainStep).toHaveAttribute("open");
    await waitFor(() => expect(sidebarStep).toHaveAttribute("open"));
    fireEvent.click(sidebarStep!.querySelector("summary")!);
    expect(sidebarStep).not.toHaveAttribute("open");
    await waitFor(() => expect(mainStep).not.toHaveAttribute("open"));
    expect(
      container.querySelector(".sidebar .nav-step .nav-file-directory"),
    ).toHaveTextContent("frontend/src/");
    expect(
      container.querySelector(".sidebar .nav-step .nav-file-name"),
    ).toHaveTextContent("App.tsx");
    expect(container.querySelector(".sidebar .nav-directory")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Files" }));

    expect(container.querySelector(".guided-group")).not.toBeInTheDocument();
    expect(container.querySelector(".files-group")).toBeInTheDocument();
    expect(
      container.querySelector(".files-group > summary h2"),
    ).toHaveTextContent("Group 1. Group title");
    expect(container.querySelector(".sidebar .nav-step")).toBeNull();
    expect(
      container.querySelector(".sidebar .nav-directory"),
    ).toHaveTextContent("frontend/src");
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

    expect(fragment).not.toHaveAttribute("open");
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    fireEvent.click(fragment!.querySelector("summary")!);
    expect(fragment).toHaveAttribute("open");
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
