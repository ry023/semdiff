import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import type {
  Anchor,
  Bootstrap,
  CategoryView,
  FileView,
  FragmentView,
  GroupView,
  ReviewLevel,
  SidebarDirectory,
  SidebarFile,
  Thread,
} from "./types";
import "./viewer.css";

const markup = (value: string) => ({ __html: value });
const list = <T,>(value: T[] | null | undefined): T[] => value ?? [];

function Importance({ value }: { value: string }) {
  return value ? (
    <span className={`importance importance-${value}`}>{value}</span>
  ) : null;
}

function Level({ value }: { value: ReviewLevel }) {
  if (!value) return null;
  return (
    <span
      className={`review-level review-level-${value}`}
      title={`${value} review`}
    >
      {value === "careful" ? "!" : value === "skim" ? "·" : "•"}
    </span>
  );
}

function StatusIcon({ html, status }: { html: string; status: string }) {
  return (
    <span
      className={`file-status-icon ${status}`}
      title={`${status} file`}
      dangerouslySetInnerHTML={markup(html)}
    />
  );
}

type FileLike = Pick<
  FileView,
  | "directory"
  | "name"
  | "status"
  | "status_icon_html"
  | "additions"
  | "deletions"
  | "diffstat"
>;

function FileHeader({ file }: { file: FileLike }) {
  return (
    <>
      <span className="file-heading">
        <StatusIcon html={file.status_icon_html} status={file.status} />
        <h3>
          {file.directory && (
            <span className="file-path">{file.directory}</span>
          )}
          <span className="file-name">{file.name}</span>
        </h3>
      </span>
      <span className="file-stats">
        <span className="stat-add">+{file.additions}</span>
        <span className="stat-del">-{file.deletions}</span>
        <span className="diffstat">
          {list(file.diffstat).map((kind, index) => (
            <span className={`diffstat-block ${kind}`} key={index} />
          ))}
        </span>
      </span>
    </>
  );
}

const sameAnchor = (a: Anchor, b: Anchor) =>
  a.type === b.type &&
  a.group_id === b.group_id &&
  a.step_id === b.step_id &&
  a.fragment_id === b.fragment_id;

interface Questions {
  mode: Bootstrap["capabilities"]["questions"];
  active: boolean;
  threads: Thread[];
  composer: Anchor | null;
  draft: string;
  error: string;
  setDraft(value: string): void;
  compose(anchor: Anchor, thread?: string): void;
  cancel(): void;
  submit(event: React.FormEvent): void;
  stop(): void;
}

function useQuestions(bootstrap: Bootstrap): Questions {
  const mode = bootstrap.capabilities.questions;
  const api = bootstrap.capabilities.api_base;
  const [threads, setThreads] = useState(bootstrap.threads ?? []);
  const [active, setActive] = useState(mode === "interactive");
  const [composer, setComposer] = useState<Anchor | null>(null);
  const [threadID, setThreadID] = useState("");
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    if (mode !== "interactive" || !api) return;
    const refresh = async () => {
      const [threadResponse, sessionResponse] = await Promise.all([
        fetch(api),
        fetch(`${api}/session`),
      ]);
      if (threadResponse.ok)
        setThreads((await threadResponse.json()) as Thread[]);
      if (sessionResponse.ok)
        setActive(
          ((await sessionResponse.json()) as { status: string }).status ===
            "active",
        );
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 2000);
    return () => window.clearInterval(timer);
  }, [api, mode]);
  return {
    mode,
    active,
    threads,
    composer,
    draft,
    error,
    setDraft,
    compose(anchor, thread = "") {
      setComposer(anchor);
      setThreadID(thread);
      setDraft("");
      setError("");
    },
    cancel() {
      setComposer(null);
      setThreadID("");
      setDraft("");
      setError("");
    },
    async submit(event) {
      event.preventDefault();
      if (!api || !composer || !draft.trim()) return;
      const body = threadID
        ? { thread_id: threadID, question: draft }
        : { anchor: composer, question: draft };
      const response = await fetch(api, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!response.ok) {
        setError(await response.text());
        return;
      }
      const updated = (await response.json()) as Thread;
      setThreads((current) => [
        ...current.filter((thread) => thread.id !== updated.id),
        updated,
      ]);
      setComposer(null);
      setThreadID("");
      setDraft("");
      setError("");
    },
    async stop() {
      if (api && (await fetch(`${api}/session`, { method: "POST" })).ok)
        setActive(false);
    },
  };
}

function Ask({ anchor, questions }: { anchor: Anchor; questions: Questions }) {
  return questions.mode === "interactive" && questions.active ? (
    <button
      className="ask-button"
      type="button"
      onClick={() => questions.compose(anchor)}
    >
      Ask
    </button>
  ) : null;
}

function DisclosureActions({ selector }: { selector: string }) {
  const setOpen = (event: React.MouseEvent, open: boolean) => {
    event.preventDefault();
    event.stopPropagation();
    const owner = event.currentTarget.closest("details");
    owner
      ?.querySelectorAll<HTMLDetailsElement>(selector)
      .forEach((details) => (details.open = open));
  };
  return (
    <span className="disclosure-actions">
      <button type="button" onClick={(event) => setOpen(event, true)}>
        Open all
      </button>
      <button type="button" onClick={(event) => setOpen(event, false)}>
        Close all
      </button>
    </span>
  );
}

function QuestionPanel({
  anchor,
  questions,
}: {
  anchor: Anchor;
  questions: Questions;
}) {
  const threads = questions.threads.filter((thread) =>
    sameAnchor(thread.anchor, anchor),
  );
  const composing =
    questions.composer && sameAnchor(questions.composer, anchor);
  if (!threads.length && !composing) return null;
  return (
    <div className="qa-panel">
      {threads.map((thread) => (
        <details className="qa-thread" key={thread.id} open>
          <summary>{list(thread.turns)[0]?.question ?? "Question"}</summary>
          {list(thread.turns).map((turn) => (
            <div className="qa-turn" key={turn.id}>
              <strong>Q</strong>
              <div dangerouslySetInnerHTML={markup(turn.question_html)} />
              {turn.answer ? (
                <>
                  <strong>A</strong>
                  <div
                    dangerouslySetInnerHTML={markup(turn.answer_html ?? "")}
                  />
                </>
              ) : (
                <>
                  <span />
                  <small>{turn.status}</small>
                </>
              )}
            </div>
          ))}
          {questions.mode === "interactive" && questions.active && (
            <button
              className="qa-follow-up"
              type="button"
              onClick={() => questions.compose(anchor, thread.id)}
            >
              Ask follow-up
            </button>
          )}
        </details>
      ))}
      {composing && (
        <form className="qa-compose" onSubmit={questions.submit}>
          <textarea
            autoFocus
            value={questions.draft}
            onChange={(event) => questions.setDraft(event.target.value)}
          />
          {questions.error && <div className="qa-error">{questions.error}</div>}
          <div className="qa-compose-actions">
            <button type="button" onClick={questions.cancel}>
              Cancel
            </button>
            <button type="submit" disabled={!questions.draft.trim()}>
              Ask
            </button>
          </div>
        </form>
      )}
    </div>
  );
}

function FragmentDescription({
  fragment,
  anchor,
  questions,
}: {
  fragment: FragmentView;
  anchor: Anchor;
  questions: Questions;
}) {
  return (
    <div className="fragment-description">
      <Level value={fragment.review_level} />
      <strong>
        {fragment.id} · {fragment.range_label}
      </strong>
      {fragment.description_html && (
        <span dangerouslySetInnerHTML={markup(fragment.description_html)} />
      )}
      <Ask anchor={anchor} questions={questions} />
    </div>
  );
}

function GuidedFragment({
  fragment,
  groupID,
  questions,
}: {
  fragment: FragmentView;
  groupID: string;
  questions: Questions;
}) {
  const anchor: Anchor = {
    type: "fragment",
    group_id: groupID,
    fragment_id: fragment.id,
  };
  const [reviewed, setReviewed] = useState(false);
  const diff =
    fragment.header_html +
    fragment.upper_context_html +
    fragment.hunk_html +
    fragment.lower_context_html;
  return (
    <details
      className={`guided-file ${reviewed ? "is-reviewed" : ""}`}
      data-group-id={groupID}
      data-file-path={fragment.path}
      open={!reviewed}
    >
      <summary>
        <FileHeader file={fragment} />
        <button
          className="file-review-toggle"
          type="button"
          aria-pressed={reviewed}
          onClick={(event) => {
            event.preventDefault();
            setReviewed(!reviewed);
          }}
        >
          ✓
        </button>
        <FragmentDescription
          fragment={fragment}
          anchor={anchor}
          questions={questions}
        />
      </summary>
      <pre dangerouslySetInnerHTML={markup(diff)} />
      <QuestionPanel anchor={anchor} questions={questions} />
    </details>
  );
}

function GuidedGroup({
  group,
  questions,
}: {
  group: GroupView;
  questions: Questions;
}) {
  const groupAnchor: Anchor = { type: "group", group_id: group.id };
  return (
    <details
      id={`guided-${group.anchor_id}`}
      className="group guided-group"
      open
    >
      <summary>
        <h2>{group.title}</h2>
        <Importance value={group.importance} />
        <span className="count">
          {list(group.steps).length} steps · {group.fragment_count} fragments
        </span>
        <DisclosureActions selector=".review-step,.guided-file" />
      </summary>
      <div
        className="summary group-summary"
        dangerouslySetInnerHTML={markup(group.summary_html)}
      />
      <Ask anchor={groupAnchor} questions={questions} />
      <QuestionPanel anchor={groupAnchor} questions={questions} />
      {list(group.steps).map((step) => {
        const anchor: Anchor = {
          type: "step",
          group_id: group.id,
          step_id: step.id,
        };
        return (
          <details
            id={step.anchor_id}
            className="review-step"
            key={step.id}
            open
          >
            <summary>
              <h3>
                {step.number}. {step.title}
              </h3>
              <div
                className="step-summary"
                dangerouslySetInnerHTML={markup(step.summary_html)}
              />
              <Ask anchor={anchor} questions={questions} />
            </summary>
            <QuestionPanel anchor={anchor} questions={questions} />
            {list(step.fragments).map((fragment) => (
              <GuidedFragment
                fragment={fragment}
                groupID={group.id}
                questions={questions}
                key={fragment.id}
              />
            ))}
          </details>
        );
      })}
    </details>
  );
}

function FileDetails({
  file,
  groupID,
  questions,
}: {
  file: FileView;
  groupID: string;
  questions: Questions;
}) {
  const [reviewed, setReviewed] = useState(false);
  const body =
    file.header_html +
    list(file.fragments)
      .map(
        (fragment) =>
          fragment.upper_context_html +
          fragment.hunk_html +
          fragment.lower_context_html,
      )
      .join("");
  return (
    <details
      id={file.anchor_id}
      className={`file main-file ${reviewed ? "is-reviewed" : ""}`}
      data-group-id={groupID}
      data-file-path={file.path}
      open
    >
      <summary>
        <FileHeader file={file} />
        <button
          className="file-review-toggle"
          type="button"
          aria-pressed={reviewed}
          onClick={(event) => {
            event.preventDefault();
            setReviewed(!reviewed);
          }}
        >
          ✓
        </button>
        {list(file.fragments).map((fragment) => (
          <FragmentDescription
            fragment={fragment}
            anchor={{
              type: "fragment",
              group_id: groupID,
              fragment_id: fragment.id,
            }}
            questions={questions}
            key={fragment.id}
          />
        ))}
      </summary>
      <pre dangerouslySetInnerHTML={markup(body)} />
      {list(file.fragments).map((fragment) => (
        <QuestionPanel
          anchor={{
            type: "fragment",
            group_id: groupID,
            fragment_id: fragment.id,
          }}
          questions={questions}
          key={fragment.id}
        />
      ))}
    </details>
  );
}

function Category({
  category,
  groupID,
  questions,
}: {
  category: CategoryView;
  groupID: string;
  questions: Questions;
}) {
  return (
    <details className="category" open>
      <summary>
        <span
          className={`category-icon ${category.icon_class}`}
          dangerouslySetInnerHTML={markup(category.icon_html)}
        />
        <strong>{category.name}</strong>
        <span className="category-stats">
          {category.added ? `${category.added} added ` : ""}
          {category.updated ? `${category.updated} updated ` : ""}
          {category.deleted ? `${category.deleted} deleted` : ""}
        </span>
        <DisclosureActions selector=":scope > .file" />
      </summary>
      {list(category.files).map((file) => (
        <FileDetails
          file={file}
          groupID={groupID}
          questions={questions}
          key={file.anchor_id}
        />
      ))}
    </details>
  );
}

function FilesGroup({
  group,
  questions,
}: {
  group: GroupView;
  questions: Questions;
}) {
  return (
    <details id={group.anchor_id} className="group files-group" open>
      <summary>
        <h2>{group.title}</h2>
        <Importance value={group.importance} />
        <span className="count">
          {list(group.files).length} files · {group.fragment_count} fragments
        </span>
        <DisclosureActions selector=".category,.file" />
      </summary>
      <div
        className="summary group-summary"
        dangerouslySetInnerHTML={markup(group.summary_html)}
      />
      {list(group.categories).map((category) => (
        <Category
          category={category}
          groupID={group.id}
          questions={questions}
          key={category.name}
        />
      ))}
    </details>
  );
}

function navigate(anchorID: string) {
  const target = document.getElementById(anchorID);
  if (!target) return;
  let parent = target.parentElement;
  while (parent) {
    if (parent instanceof HTMLDetailsElement) parent.open = true;
    parent = parent.parentElement;
  }
  if (target instanceof HTMLDetailsElement) target.open = true;
  history.replaceState(null, "", `#${anchorID}`);
  target.scrollIntoView({ block: "start" });
}

function SidebarFileNode({ file }: { file: SidebarFile }) {
  return (
    <details className="nav-file-node" open>
      <summary>
        <span dangerouslySetInnerHTML={markup(file.status_icon_html)} />
        <span>{file.name}</span>
        <Level value={file.review_level} />
      </summary>
      {list(file.occurrences).map((occurrence) => (
        <button
          className="nav-link occurrence"
          type="button"
          onClick={() => navigate(occurrence.file_anchor_id)}
          key={`${occurrence.group_id}-${occurrence.file_anchor_id}`}
        >
          {occurrence.group_title}
          <small>{occurrence.fragment_count} fragments</small>
        </button>
      ))}
    </details>
  );
}

function Directory({ directory }: { directory: SidebarDirectory }) {
  return (
    <details className="nav-directory" open>
      <summary>
        <span>▰</span>
        <span>{directory.name}</span>
        <small>{directory.file_count}</small>
      </summary>
      <div className="nav-children">
        {list(directory.directories).map((child) => (
          <Directory directory={child} key={child.name} />
        ))}
        {list(directory.files).map((file) => (
          <SidebarFileNode file={file} key={file.path} />
        ))}
      </div>
    </details>
  );
}

function Sidebar({
  bootstrap,
  activeKey,
}: {
  bootstrap: Bootstrap;
  activeKey: string;
}) {
  const [tab, setTab] = useState<"groups" | "files">("groups");
  const [width, setWidth] = useState(() => {
    try {
      return Number(localStorage.getItem("semdiff-sidebar-width")) || 360;
    } catch {
      return 360;
    }
  });
  const [resizing, setResizing] = useState(false);
  useEffect(() => {
    if (!resizing) return;
    const move = (event: PointerEvent) =>
      setWidth(Math.max(260, Math.min(640, event.clientX)));
    const finish = () => {
      setResizing(false);
      try {
        localStorage.setItem("semdiff-sidebar-width", String(width));
      } catch {
        // Persistence is optional in private browsing contexts.
      }
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", finish, { once: true });
    return () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", finish);
    };
  }, [resizing, width]);
  return (
    <aside className="sidebar" style={{ width }}>
      <div className="sidebar-tabs">
        <button
          aria-selected={tab === "groups"}
          onClick={() => setTab("groups")}
        >
          Groups
        </button>
        <button aria-selected={tab === "files"} onClick={() => setTab("files")}>
          Files
        </button>
      </div>
      <nav className="sidebar-pane">
        {tab === "groups" ? (
          list(bootstrap.page.groups).map((group) => (
            <details className="nav-group" key={group.id} open>
              <summary>
                <span>{group.title}</span>
                <Importance value={group.importance} />
                <small>{list(group.files).length}</small>
              </summary>
              {list(group.files).map((file) => (
                <button
                  className={`nav-link ${activeKey === `${group.id}\0${file.path}` ? "is-active" : ""}`}
                  type="button"
                  onClick={() => navigate(file.anchor_id)}
                  key={file.anchor_id}
                >
                  {file.path}
                  <small>
                    <span className="stat-add">+{file.additions}</span>{" "}
                    <span className="stat-del">−{file.deletions}</span>
                  </small>
                </button>
              ))}
            </details>
          ))
        ) : (
          <>
            {list(bootstrap.page.sidebar_directories).map((directory) => (
              <Directory directory={directory} key={directory.name} />
            ))}
            {list(bootstrap.page.sidebar_files).map((file) => (
              <SidebarFileNode file={file} key={file.path} />
            ))}
          </>
        )}
      </nav>
      <div
        className="sidebar-resize-handle"
        onPointerDown={(event) => {
          event.preventDefault();
          setResizing(true);
        }}
      />
    </aside>
  );
}

function Drift({ bootstrap }: { bootstrap: Bootstrap }) {
  const drift = bootstrap.page.drift;
  if (!drift) return null;
  return (
    <section className="review-drift">
      <strong>
        This semantic review is {list(drift.commits).length} unreviewed commits
        behind HEAD.
      </strong>
      <p>
        Groups cover{" "}
        <code>
          {bootstrap.page.base_sha}...{bootstrap.page.head_sha}
        </code>
        . Current range:{" "}
        <code>
          {drift.current_base_sha}...{drift.current_head_sha}
        </code>
        .
      </p>
      <details>
        <summary>Changes since this review</summary>
        <div className="drift-columns">
          <ul>
            {list(drift.commits).map((commit) => (
              <li key={commit.sha}>
                <code>{commit.sha.slice(0, 12)}</code> {commit.subject}
              </li>
            ))}
          </ul>
          <ul>
            {list(drift.paths).map((path) => (
              <li key={path}>
                <code>{path}</code>
              </li>
            ))}
          </ul>
        </div>
      </details>
    </section>
  );
}

function App({ bootstrap }: { bootstrap: Bootstrap }) {
  const [reviewMode, setReviewMode] = useState<"guided" | "files">("guided");
  const [diffMode, setDiffMode] = useState<"unified" | "split">("unified");
  const [activeKey, setActiveKey] = useState("");
  const questions = useQuestions(bootstrap);
  useEffect(() => {
    document.body.dataset.view = diffMode;
  }, [diffMode]);
  useEffect(() => {
    if (location.hash)
      requestAnimationFrame(() => navigate(location.hash.slice(1)));
  }, [reviewMode]);
  useEffect(() => {
    const expand = (event: MouseEvent) => {
      const button = (
        event.target as Element | null
      )?.closest<HTMLButtonElement>(".expand-lines");
      const container = button?.closest(".context-expand");
      if (!button || !container) return;
      const hidden = Array.from(
        container.querySelectorAll<HTMLElement>(".context-hidden[hidden]"),
      );
      (button.dataset.direction === "up"
        ? hidden.slice(-10)
        : hidden.slice(0, 10)
      ).forEach((line) => {
        line.hidden = false;
      });
      if (!container.querySelector(".context-hidden[hidden]"))
        container
          .querySelectorAll(".expand-lines")
          .forEach((item) => item.remove());
    };
    document.addEventListener("click", expand);
    return () => document.removeEventListener("click", expand);
  }, []);
  useEffect(() => {
    let scheduled = false;
    const sync = () => {
      if (scheduled) return;
      scheduled = true;
      requestAnimationFrame(() => {
        scheduled = false;
        const visible = Array.from(
          document.querySelectorAll<HTMLElement>(
            ".main-file,.guided-file[data-group-id]",
          ),
        ).filter((file) => {
          const rect = file.getBoundingClientRect();
          return (
            file.offsetParent !== null &&
            rect.bottom > 48 &&
            rect.top < innerHeight
          );
        });
        visible.sort(
          (left, right) =>
            Math.abs(left.getBoundingClientRect().top - 80) -
            Math.abs(right.getBoundingClientRect().top - 80),
        );
        const file = visible[0];
        if (file)
          setActiveKey(`${file.dataset.groupId}\0${file.dataset.filePath}`);
      });
    };
    window.addEventListener("scroll", sync, { passive: true });
    sync();
    return () => window.removeEventListener("scroll", sync);
  }, [reviewMode]);
  return (
    <div className="app-shell">
      <Sidebar bootstrap={bootstrap} activeKey={activeKey} />
      <main className="wrap">
        <header className="page-header">
          <div>
            <h1>Semantic Changes</h1>
            <code>
              {bootstrap.page.base_sha} → {bootstrap.page.head_sha}
            </code>
          </div>
          {questions.mode === "interactive" && (
            <button
              type="button"
              disabled={!questions.active}
              onClick={questions.stop}
            >
              {questions.active ? "End answer mode" : "Answer mode stopped"}
            </button>
          )}
        </header>
        <div className="stats">
          {list(bootstrap.page.groups).length} groups ·{" "}
          {bootstrap.page.file_count} files · {bootstrap.page.fragment_count}{" "}
          fragments
        </div>
        <Drift bootstrap={bootstrap} />
        <div className="toolbar">
          <div>
            {(["guided", "files"] as const).map((mode) => (
              <button
                key={mode}
                aria-pressed={reviewMode === mode}
                onClick={() => setReviewMode(mode)}
              >
                {mode === "guided" ? "Guided" : "Files"}
              </button>
            ))}
          </div>
          <div>
            {(["unified", "split"] as const).map((mode) => (
              <button
                key={mode}
                aria-pressed={diffMode === mode}
                onClick={() => setDiffMode(mode)}
              >
                {mode === "unified" ? "Unified" : "Split"}
              </button>
            ))}
          </div>
        </div>
        {list(bootstrap.page.groups).map((group) =>
          reviewMode === "guided" ? (
            <GuidedGroup group={group} questions={questions} key={group.id} />
          ) : (
            <FilesGroup group={group} questions={questions} key={group.id} />
          ),
        )}
      </main>
    </div>
  );
}

const root = document.getElementById("root");
const data = document.getElementById("semdiff-data");
if (root && data?.textContent) {
  try {
    createRoot(root).render(
      <App bootstrap={JSON.parse(data.textContent) as Bootstrap} />,
    );
  } catch (error) {
    createRoot(root).render(
      <main className="bootstrap-error">
        <h1>Semantic Changes</h1>
        <p>Viewer data is invalid: {String(error)}</p>
      </main>,
    );
  }
}
