import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Bootstrap, DiffItem, FragmentView } from "./types";
import { App, expandContext, syncStickyOffsets } from "./main";
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
        "1 Group · 1 File · 1 Fragment",
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
      ).toHaveTextContent("logic");
      fireEvent.click(screen.getByRole("button", { name: "Files" }));
      expect(
        container.querySelector(".files-group > summary .count"),
      ).toHaveTextContent("1 File · 1 Fragment");
      expect(
        container.querySelector(".files-group .file-status-icon"),
      ).toHaveAttribute("title", "Updated File");
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

  it("keeps disclosure controls in each group header and scopes their action", async () => {
    const { container } = render(<App bootstrap={bootstrap()} />);
    const header = container.querySelector(".viewer-shell > .page-header")!;
    const guidedGroup = container.querySelector(".guided-group")!;
    const step = container.querySelector(".review-step")!;
    const guidedFile = container.querySelector(".guided-file")!;
    const sidebarStep = container.querySelector(".sidebar .nav-step")!;
    const guidedSummary =
      guidedGroup.querySelector<HTMLElement>(":scope > summary")!;
    const guidedOpenAll = within(guidedSummary).getByRole("button", {
      name: "Open all",
    });
    const guidedCloseAll = within(guidedSummary).getByRole("button", {
      name: "Close all",
    });

    expect(header).not.toContainElement(guidedOpenAll);
    expect(header).not.toContainElement(guidedCloseAll);
    fireEvent.click(guidedOpenAll);
    expect(guidedGroup).toHaveAttribute("open");
    expect(guidedFile).toHaveAttribute("open");
    await waitFor(() => {
      expect(step).toHaveAttribute("open");
      expect(sidebarStep).toHaveAttribute("open");
    });

    fireEvent.click(guidedCloseAll);
    expect(guidedGroup).toHaveAttribute("open");
    expect(guidedFile).not.toHaveAttribute("open");
    await waitFor(() => {
      expect(step).not.toHaveAttribute("open");
      expect(sidebarStep).not.toHaveAttribute("open");
    });

    fireEvent.click(screen.getByRole("button", { name: "Files" }));
    const filesGroup = container.querySelector(".files-group")!;
    const category = container.querySelector(".category")!;
    const file = container.querySelector(".main-file")!;
    const filesSummary =
      filesGroup.querySelector<HTMLElement>(":scope > summary")!;
    const filesOpenAll = within(filesSummary).getByRole("button", {
      name: "Open all",
    });
    const filesCloseAll = within(filesSummary).getByRole("button", {
      name: "Close all",
    });
    fireEvent.click(filesOpenAll);
    expect(filesGroup).toHaveAttribute("open");
    expect(category).toHaveAttribute("open");
    expect(file).toHaveAttribute("open");

    fireEvent.click(filesCloseAll);
    expect(filesGroup).toHaveAttribute("open");
    expect(category).not.toHaveAttribute("open");
    expect(file).not.toHaveAttribute("open");
  });

  it("renders guided and sidebar category markers without fragment internals", () => {
    const { container } = render(<App bootstrap={bootstrap()} />);

    expect(
      container.querySelector(".guided-file .category-badge"),
    ).toHaveTextContent("logic");
    expect(
      container.querySelector(".nav-group-file .category-icon"),
    ).toHaveAttribute("aria-label", "Category: logic");
    const sidebarFragment = container.querySelector(".nav-guided-fragment")!;
    expect(sidebarFragment.children[1]).toHaveClass("review-level");
    expect(sidebarFragment.children[2]).toHaveClass("nav-file-path");
    expect(sidebarFragment.children[3]).toHaveClass("category-icon");
    expect(screen.getAllByText("App.tsx").length).toBeGreaterThan(0);
    expect(screen.queryByText("F1")).not.toBeInTheDocument();
    expect(screen.queryByText("L10-L20")).not.toBeInTheDocument();
    expect(screen.queryByText(/diff --git/)).not.toBeInTheDocument();
  });

  it("explains importance, review level, and category on keyboard focus", async () => {
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

    const categoryBadge = container.querySelector<HTMLElement>(
      ".guided-file .category-badge",
    )!;
    categoryBadge.focus();
    await waitFor(() =>
      expect(screen.getByRole("tooltip")).toHaveTextContent("Category: logic"),
    );

    const sidebarCategory = container.querySelector<HTMLElement>(
      ".nav-guided-fragment .category-icon",
    )!;
    sidebarCategory.focus();
    await waitFor(() =>
      expect(screen.getByRole("tooltip")).toHaveTextContent("Category: logic"),
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

  it("keeps Group, Step, and Fragment headers sticky in the stack", () => {
    const { container } = render(<App bootstrap={bootstrap()} />);
    const group = container.querySelector(".guided-group")!;
    const groupHeader = group.querySelector<HTMLElement>(":scope > summary")!;
    const step = container.querySelector(".review-step")!;
    const stepHeader = step.querySelector<HTMLElement>(":scope > summary")!;
    const fragment = container.querySelector(".guided-file")!;
    const fragmentHeader =
      fragment.querySelector<HTMLElement>(":scope > summary")!;

    expect(getComputedStyle(groupHeader).position).toBe("sticky");
    expect(getComputedStyle(stepHeader).position).toBe("sticky");
    expect(getComputedStyle(fragmentHeader).position).toBe("static");

    fireEvent.click(
      within(groupHeader).getByRole("button", { name: "Open all" }),
    );

    expect(fragment).toHaveAttribute("open");
    expect(getComputedStyle(fragmentHeader).position).toBe("sticky");
    expect(getComputedStyle(fragment).scrollMarginTop).toBe(
      "var(--diff-header-top)",
    );
    expect(getComputedStyle(fragmentHeader).top).toBe("var(--diff-header-top)");
  });

  it("connects Guided Fragment and Files headers directly to their Diffs", () => {
    const { container } = render(<App bootstrap={bootstrap()} />);
    const groupHeader = container.querySelector<HTMLElement>(
      ".guided-group > summary",
    )!;
    const fragment = container.querySelector(".guided-file")!;
    expect(fragment).not.toHaveAttribute("open");
    expect(getComputedStyle(fragment).marginTop).toBe("0");
    expect(getComputedStyle(fragment).padding).toBe("8px");

    fireEvent.click(
      within(groupHeader).getByRole("button", { name: "Open all" }),
    );

    const header = fragment.querySelector(":scope > summary")!;
    const diff = fragment.querySelector(":scope > pre")!;

    expect(fragment).toHaveAttribute("open");
    expect(getComputedStyle(fragment).padding).toBe("0px");
    expect(getComputedStyle(header).margin).toBe("0px");
    expect(getComputedStyle(diff).marginTop).toBe("0px");
    expect(getComputedStyle(diff).paddingTop).toBe("0px");
    expect(getComputedStyle(diff).paddingBottom).toBe("0px");

    fireEvent.click(screen.getByRole("button", { name: "Files" }));
    const file = container.querySelector(".main-file")!;
    const fileHeader = file.querySelector<HTMLElement>(":scope > summary")!;
    const fileDiff = file.querySelector(":scope > pre")!;

    expect(fragment).toHaveClass("diff-view");
    expect(file).toHaveClass("diff-view");
    expect(file).toHaveAttribute("open");
    expect(getComputedStyle(file).scrollMarginTop).toBe(
      "var(--diff-header-top)",
    );
    expect(getComputedStyle(fileHeader).top).toBe("var(--diff-header-top)");
    expect(getComputedStyle(file).padding).toBe("0px");
    expect(getComputedStyle(fileHeader).margin).toBe("0px");
    expect(getComputedStyle(fileDiff).marginTop).toBe("0px");
    expect(getComputedStyle(fileDiff).paddingTop).toBe("0px");
    expect(getComputedStyle(fileDiff).paddingBottom).toBe("0px");
  });

  it("measures wrapped sticky headers into their descendant offsets", () => {
    const shell = document.createElement("div");
    shell.innerHTML = `
      <header class="page-header"></header>
      <details class="group"><summary></summary>
        <details class="review-step"><summary></summary>
          <details class="guided-file"><summary></summary></details>
        </details>
        <details class="category"><summary></summary>
          <details class="file"><summary></summary></details>
        </details>
      </details>`;
    const setHeight = (element: Element | null, height: number) => {
      Object.defineProperty(element, "getBoundingClientRect", {
        configurable: true,
        value: () => ({ height }),
      });
    };
    setHeight(shell.querySelector(":scope > .page-header"), 76.25);
    setHeight(shell.querySelector(".group > summary"), 68.5);
    setHeight(shell.querySelector(".review-step > summary"), 52.75);
    setHeight(shell.querySelector(".category > summary"), 48.125);

    syncStickyOffsets(shell);

    expect(shell.style.getPropertyValue("--page-header-height")).toBe(
      "76.25px",
    );
    expect(
      shell
        .querySelector<HTMLElement>(".group")
        ?.style.getPropertyValue("--group-header-height"),
    ).toBe("68.5px");
    expect(
      shell
        .querySelector<HTMLElement>(".review-step")
        ?.style.getPropertyValue("--step-header-height"),
    ).toBe("52.75px");
    expect(
      shell
        .querySelector<HTMLElement>(".category")
        ?.style.getPropertyValue("--category-header-height"),
    ).toBe("48.125px");
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
    fireEvent.click(
      within(sidebarStep!.querySelector("summary")!).getByRole("button", {
        name: "Toggle Step",
      }),
    );
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

  it("uses sidebar chevrons for disclosure and header text for navigation", async () => {
    const { container } = render(<App bootstrap={bootstrap()} />);
    const mainGroup = container.querySelector<HTMLElement>(".guided-group")!;
    const mainStep = container.querySelector<HTMLElement>(".review-step")!;
    const sidebarGroup = container.querySelector<HTMLDetailsElement>(
      ".sidebar .nav-group",
    )!;
    const sidebarStep =
      container.querySelector<HTMLDetailsElement>(".sidebar .nav-step")!;
    mainGroup.scrollIntoView = vi.fn();
    mainStep.scrollIntoView = vi.fn();

    fireEvent.click(sidebarGroup.querySelector(":scope > summary > span")!);
    expect(sidebarGroup).toHaveAttribute("open");
    expect(location.hash).toBe("#guided-group-0");
    expect(mainGroup.scrollIntoView).toHaveBeenCalled();

    fireEvent.click(sidebarStep.querySelector(":scope > summary > span")!);
    expect(sidebarStep).not.toHaveAttribute("open");
    expect(mainStep).not.toHaveAttribute("open");
    expect(location.hash).toBe("#step-0-0");
    expect(mainStep.scrollIntoView).toHaveBeenCalled();

    fireEvent.click(
      within(sidebarStep.querySelector(":scope > summary")!).getByRole(
        "button",
        { name: "Toggle Step" },
      ),
    );
    await waitFor(() => {
      expect(sidebarStep).toHaveAttribute("open");
      expect(mainStep).toHaveAttribute("open");
    });

    fireEvent.click(
      within(sidebarGroup.querySelector(":scope > summary")!).getByRole(
        "button",
        { name: "Toggle Group" },
      ),
    );
    expect(sidebarGroup).not.toHaveAttribute("open");

    fireEvent.click(screen.getByRole("button", { name: "Files" }));
    const filesGroup = container.querySelector<HTMLElement>(".files-group")!;
    const filesSidebarGroup = container.querySelector<HTMLDetailsElement>(
      ".sidebar .nav-group",
    )!;
    filesGroup.scrollIntoView = vi.fn();

    fireEvent.click(
      filesSidebarGroup.querySelector(":scope > summary > span")!,
    );
    expect(filesSidebarGroup).toHaveAttribute("open");
    expect(location.hash).toBe("#group-0");
    expect(filesGroup.scrollIntoView).toHaveBeenCalled();

    fireEvent.click(
      within(filesSidebarGroup.querySelector(":scope > summary")!).getByRole(
        "button",
        { name: "Toggle Group" },
      ),
    );
    expect(filesSidebarGroup).not.toHaveAttribute("open");
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
