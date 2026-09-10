import React, { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import hljs from "highlight.js/lib/core";
import bash from "highlight.js/lib/languages/bash";
import cpp from "highlight.js/lib/languages/cpp";
import csharp from "highlight.js/lib/languages/csharp";
import css from "highlight.js/lib/languages/css";
import go from "highlight.js/lib/languages/go";
import java from "highlight.js/lib/languages/java";
import javascript from "highlight.js/lib/languages/javascript";
import json from "highlight.js/lib/languages/json";
import kotlin from "highlight.js/lib/languages/kotlin";
import python from "highlight.js/lib/languages/python";
import ruby from "highlight.js/lib/languages/ruby";
import rust from "highlight.js/lib/languages/rust";
import scss from "highlight.js/lib/languages/scss";
import sql from "highlight.js/lib/languages/sql";
import typescript from "highlight.js/lib/languages/typescript";
import xml from "highlight.js/lib/languages/xml";
import {
  Check,
  ChevronRight,
  Circle,
  CircleAlert,
  CircleCheck,
  CircleDashed,
  CircleHelp,
  Code2,
  FileCheck,
  FileMinus,
  FilePlus,
  FileText,
  Folder,
  GitBranch,
  PanelTop,
  Settings,
  Tag,
  type LucideIcon,
} from "lucide-react";
import type {
  Anchor,
  Bootstrap,
  CategoryView,
  DiffItem,
  FileView,
  FragmentView,
  GroupView,
  ReviewLevel,
  Thread,
} from "./types";
import "./viewer.css";

for (const [name, language] of Object.entries({
  bash,
  cpp,
  csharp,
  css,
  go,
  java,
  javascript,
  json,
  kotlin,
  python,
  ruby,
  rust,
  scss,
  sql,
  typescript,
  xml,
})) {
  hljs.registerLanguage(name, language);
}

const markup = (value: string) => ({ __html: value });
const list = <T,>(value: T[] | null | undefined): T[] => value ?? [];

function Importance({ value }: { value: string }) {
  return value ? (
    <span className={`importance importance-${value}`}>{value}</span>
  ) : null;
}

function Level({ value }: { value: ReviewLevel }) {
  if (!value) return null;
  const Icon =
    value === "careful"
      ? CircleAlert
      : value === "skim"
        ? CircleDashed
        : Circle;
  return (
    <span
      className={`review-level review-level-${value}`}
      title={`${value} review`}
    >
      <Icon size={16} aria-hidden="true" />
    </span>
  );
}

function StatusIcon({ status }: { status: string }) {
  const Icon =
    status === "new" ? FilePlus : status === "deleted" ? FileMinus : FileCheck;
  return (
    <span
      className={`file-status-icon ${status}`}
      title={`${status} file`}
      aria-hidden="true"
    >
      <Icon size={20} />
    </span>
  );
}

const categoryIcons: Record<string, LucideIcon> = {
  implementation: Code2,
  test: CircleCheck,
  component: PanelTop,
  logic: GitBranch,
  config: Settings,
  docs: FileText,
  unknown: CircleHelp,
  custom: Tag,
};

function CategoryIcon({ name }: { name: string }) {
  const Icon = categoryIcons[name] ?? Tag;
  return (
    <span className="category-icon" aria-hidden="true">
      <Icon size={18} />
    </span>
  );
}

function DisclosureIcon() {
  return (
    <ChevronRight className="disclosure-icon" size={16} aria-hidden="true" />
  );
}

type FileLike = Pick<
  FileView,
  "directory" | "name" | "status" | "additions" | "deletions" | "diffstat"
>;

function FileHeader({ file }: { file: FileLike }) {
  return (
    <>
      <span className="file-heading">
        <StatusIcon status={file.status} />
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

const languageByExtension: Record<string, string> = {
  bash: "bash",
  c: "cpp",
  cc: "cpp",
  cpp: "cpp",
  cs: "csharp",
  css: "css",
  go: "go",
  h: "cpp",
  hpp: "cpp",
  htm: "xml",
  html: "xml",
  java: "java",
  js: "javascript",
  json: "json",
  jsx: "javascript",
  kt: "kotlin",
  mjs: "javascript",
  py: "python",
  rb: "ruby",
  rs: "rust",
  scss: "scss",
  sh: "bash",
  sql: "sql",
  ts: "typescript",
  tsx: "typescript",
  vue: "xml",
  xml: "xml",
  zsh: "bash",
};

function languageForPath(path: string): string | undefined {
  const name = path.split("/").pop() ?? path;
  const extension = name.includes(".") ? name.split(".").pop() : undefined;
  return extension ? languageByExtension[extension.toLowerCase()] : undefined;
}

const escapeHTML = (value: string) =>
  value.replace(
    /[&<>"']/g,
    (character) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        character
      ] ?? character,
  );

function highlightDiffText(item: DiffItem, path: string): string {
  const line = item.text ?? "";
  if (item.class === "meta") return escapeHTML(line);
  let prefix = "";
  let source = line;
  if (
    (item.class === "add" ||
      item.class === "del" ||
      item.class?.startsWith("ctx")) &&
    source.length > 0
  ) {
    prefix = source[0];
    source = source.slice(1);
  }
  const language = languageForPath(path);
  let highlighted = escapeHTML(source);
  if (language && hljs.getLanguage(language)) {
    try {
      highlighted = hljs.highlight(source, { language }).value;
    } catch {
      // An unknown or malformed source line should remain safely visible.
    }
  }
  return escapeHTML(prefix) + highlighted;
}

function DiffItems({
  items,
  path,
}: {
  items: DiffItem[] | null;
  path: string;
}) {
  const renderItem = (item: DiffItem, index: number) => {
    if (item.kind === "expand") {
      const direction = item.direction === "up" ? "up" : "down";
      return (
        <button
          className="expand-lines"
          type="button"
          data-direction={direction}
          key={`expand-${index}`}
        >
          {direction === "up" ? "↑" : "↓"} Show {item.count ?? 0} lines{" "}
          {direction === "up" ? "above" : "below"}
        </button>
      );
    }
    const rowClass = item.class ?? "ctx";
    const highlighted = highlightDiffText(item, path);
    const oldNumber = item.old_number ?? "";
    const newNumber = item.new_number ?? "";
    const unifiedNumber = newNumber || oldNumber;
    const splitCode = (side: "old" | "new") => {
      if (rowClass === "add") return side === "new" ? highlighted : "";
      if (rowClass === "del") return side === "old" ? highlighted : "";
      return highlighted;
    };
    return (
      <span
        className={`diff-row ${rowClass}${item.hidden ? " context-hidden" : ""}`}
        hidden={item.hidden || undefined}
        key={`line-${index}`}
      >
        <span className="line-number unified-cell">{unifiedNumber}</span>
        <span
          className="line-code unified-cell"
          dangerouslySetInnerHTML={markup(highlighted)}
        />
        {oldNumber === "" && newNumber === "" ? (
          <span
            className="split-wide"
            dangerouslySetInnerHTML={markup(highlighted)}
          />
        ) : (
          <>
            <span className="line-number split-cell old-number">
              {oldNumber}
            </span>
            <span
              className="line-code split-cell old-code"
              dangerouslySetInnerHTML={markup(splitCode("old"))}
            />
            <span className="line-number split-cell new-number">
              {newNumber}
            </span>
            <span
              className="line-code split-cell new-code"
              dangerouslySetInnerHTML={markup(splitCode("new"))}
            />
          </>
        )}
      </span>
    );
  };
  const groups: Array<{ context: DiffItem["context"]; items: DiffItem[] }> = [];
  for (const item of list(items)) {
    const previous = groups[groups.length - 1];
    if (previous && previous.context === item.context) {
      previous.items.push(item);
    } else {
      groups.push({ context: item.context, items: [item] });
    }
  }
  return (
    <>
      {groups.map((group, groupIndex) => {
        const content = group.items.map((item, index) =>
          renderItem(item, index),
        );
        return group.context ? (
          <span
            className={`context-expand context-${group.context}`}
            key={`context-${groupIndex}`}
          >
            {content}
          </span>
        ) : (
          <React.Fragment key={`items-${groupIndex}`}>{content}</React.Fragment>
        );
      })}
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
  const initialThreads = bootstrap.threads ?? [];
  const [threads, setThreads] = useState(initialThreads);
  const threadsFingerprint = useRef(JSON.stringify(initialThreads));
  const sessionStatus = useRef(mode === "interactive" ? "active" : "stopped");
  const [active, setActive] = useState(mode === "interactive");
  const [composer, setComposer] = useState<Anchor | null>(null);
  const [threadID, setThreadID] = useState("");
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    if (mode !== "interactive" || !api) return;
    let refreshing = false;
    let disposed = false;
    const refresh = async () => {
      if (refreshing) return;
      refreshing = true;
      try {
        const [threadResponse, sessionResponse] = await Promise.all([
          fetch(api),
          fetch(`${api}/session`),
        ]);
        if (threadResponse.ok) {
          const nextThreads = (await threadResponse.json()) as Thread[];
          const nextFingerprint = JSON.stringify(nextThreads);
          if (!disposed && nextFingerprint !== threadsFingerprint.current) {
            threadsFingerprint.current = nextFingerprint;
            setThreads(nextThreads);
          }
        }
        if (sessionResponse.ok) {
          const nextStatus = (
            (await sessionResponse.json()) as { status: string }
          ).status;
          if (!disposed && nextStatus !== sessionStatus.current) {
            sessionStatus.current = nextStatus;
            setActive(nextStatus === "active");
          }
        }
      } finally {
        refreshing = false;
      }
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 2000);
    return () => {
      disposed = true;
      window.clearInterval(timer);
    };
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
          <summary>
            <DisclosureIcon />
            {list(thread.turns)[0]?.question ?? "Question"}
          </summary>
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
  const diff = [
    ...list(fragment.header),
    ...list(fragment.upper_context),
    ...list(fragment.hunk),
    ...list(fragment.lower_context),
  ];
  return (
    <details
      className={`guided-file ${reviewed ? "is-reviewed" : ""}`}
      data-group-id={groupID}
      data-file-path={fragment.path}
      open={!reviewed}
    >
      <summary>
        <DisclosureIcon />
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
          <Check size={16} aria-hidden="true" />
        </button>
        <FragmentDescription
          fragment={fragment}
          anchor={anchor}
          questions={questions}
        />
      </summary>
      <pre>
        <DiffItems items={diff} path={fragment.path} />
      </pre>
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
        <DisclosureIcon />
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
              <DisclosureIcon />
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
  const body = [
    ...list(file.header),
    ...list(file.fragments).flatMap((fragment) => [
      ...list(fragment.upper_context),
      ...list(fragment.hunk),
      ...list(fragment.lower_context),
    ]),
  ];
  return (
    <details
      id={file.anchor_id}
      className={`file main-file ${reviewed ? "is-reviewed" : ""}`}
      data-group-id={groupID}
      data-file-path={file.path}
      open
    >
      <summary>
        <DisclosureIcon />
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
          <Check size={16} aria-hidden="true" />
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
      <pre>
        <DiffItems items={body} path={file.path} />
      </pre>
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
        <DisclosureIcon />
        <CategoryIcon name={category.icon} />
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
        <DisclosureIcon />
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

interface FileTreeNode {
  name: string;
  directories: FileTreeNode[];
  files: FileView[];
}

function buildFileTree(files: FileView[]): FileTreeNode {
  const root: FileTreeNode = { name: "", directories: [], files: [] };
  for (const file of files) {
    const parts = file.path.split("/");
    let parent = root;
    for (const part of parts.slice(0, -1)) {
      let directory = parent.directories.find((item) => item.name === part);
      if (!directory) {
        directory = { name: part, directories: [], files: [] };
        parent.directories.push(directory);
      }
      parent = directory;
    }
    parent.files.push(file);
  }
  root.directories.forEach(compressFileTree);
  return root;
}

function compressFileTree(directory: FileTreeNode): void {
  directory.directories.forEach(compressFileTree);
  while (directory.files.length === 0 && directory.directories.length === 1) {
    const child = directory.directories[0];
    directory.name = `${directory.name}/${child.name}`;
    directory.directories = child.directories;
    directory.files = child.files;
  }
}

function fileTreeCount(directory: FileTreeNode): number {
  return (
    directory.files.length +
    directory.directories.reduce(
      (count, child) => count + fileTreeCount(child),
      0,
    )
  );
}

function GroupFileLink({
  file,
  groupID,
  activeKey,
}: {
  file: FileView;
  groupID: string;
  activeKey: string;
}) {
  return (
    <button
      className={`nav-link nav-group-file ${activeKey === `${groupID}\0${file.path}` ? "is-active" : ""}`}
      type="button"
      onClick={() => navigate(file.anchor_id)}
      key={file.anchor_id}
    >
      <StatusIcon status={file.status} />
      <span className="nav-file-name">{file.name}</span>
      <small>
        <span className="stat-add">+{file.additions}</span>{" "}
        <span className="stat-del">−{file.deletions}</span>
      </small>
    </button>
  );
}

function GroupDirectory({
  directory,
  groupID,
  activeKey,
}: {
  directory: FileTreeNode;
  groupID: string;
  activeKey: string;
}) {
  return (
    <details className="nav-directory" open>
      <summary>
        <DisclosureIcon />
        <Folder size={16} aria-hidden="true" />
        <span>{directory.name}</span>
        <small>{fileTreeCount(directory)}</small>
      </summary>
      <div className="nav-children">
        {directory.directories.map((child) => (
          <GroupDirectory
            directory={child}
            groupID={groupID}
            activeKey={activeKey}
            key={child.name}
          />
        ))}
        {directory.files.map((file) => (
          <GroupFileLink
            file={file}
            groupID={groupID}
            activeKey={activeKey}
            key={file.path}
          />
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
      <nav className="sidebar-pane">
        {list(bootstrap.page.groups).map((group) => {
          const tree = buildFileTree(list(group.files));
          return (
            <details className="nav-group" key={group.id} open>
              <summary>
                <DisclosureIcon />
                <span>{group.title}</span>
                <Importance value={group.importance} />
                <small>{list(group.files).length}</small>
              </summary>
              <div className="nav-group-files">
                {tree.directories.map((directory) => (
                  <GroupDirectory
                    directory={directory}
                    groupID={group.id}
                    activeKey={activeKey}
                    key={directory.name}
                  />
                ))}
                {tree.files.map((file) => (
                  <GroupFileLink
                    file={file}
                    groupID={group.id}
                    activeKey={activeKey}
                    key={file.path}
                  />
                ))}
              </div>
            </details>
          );
        })}
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
        <summary>
          <DisclosureIcon />
          Changes since this review
        </summary>
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
