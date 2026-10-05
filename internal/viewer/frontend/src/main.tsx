import React, { useContext, useEffect, useRef, useState } from "react";
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
import {
  categoryName,
  detectLocale,
  LocaleContext,
  t,
  type Locale,
  type MessageKey,
  useLocale,
} from "./i18n";
import { Markdown } from "./markdown";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "./tooltip";
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
const DiffModeContext = React.createContext<"unified" | "split">("unified");
const maxHighlightedLineLength = 10_000;
const maxRenderedLineLength = 20_000;
const pluralTerm = (count: number, term: string) =>
  `${term}${count === 1 ? "" : "s"}`;

const importanceMessageKeys: Record<
  string,
  { label: MessageKey; description: MessageKey }
> = {
  core: { label: "importanceCore", description: "importanceCoreDescription" },
  supporting: {
    label: "importanceSupporting",
    description: "importanceSupportingDescription",
  },
  side: { label: "importanceSide", description: "importanceSideDescription" },
};
const reviewMessageKeys: Record<
  ReviewLevel,
  { label: MessageKey; description: MessageKey }
> = {
  careful: { label: "reviewCareful", description: "reviewCarefulDescription" },
  normal: { label: "reviewNormal", description: "reviewNormalDescription" },
  skim: { label: "reviewSkim", description: "reviewSkimDescription" },
  "": { label: "reviewNormal", description: "reviewNormalDescription" },
};

function Importance({ value }: { value: string }) {
  const locale = useLocale();
  const keys = importanceMessageKeys[value];
  return value ? (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={`importance importance-${value}`}
          tabIndex={0}
          onClick={(event) => event.preventDefault()}
        >
          {keys ? t(locale, keys.label) : value}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <strong>{keys ? t(locale, keys.label) : value}</strong> —{" "}
        {keys ? t(locale, keys.description) : value}
      </TooltipContent>
    </Tooltip>
  ) : null;
}

function Level({ value }: { value: ReviewLevel }) {
  if (!value) return null;
  const locale = useLocale();
  const keys = reviewMessageKeys[value];
  const Icon =
    value === "careful"
      ? CircleAlert
      : value === "skim"
        ? CircleDashed
        : Circle;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={`review-level review-level-${value}`}
          tabIndex={0}
          aria-label={t(locale, keys.label)}
          onClick={(event) => event.preventDefault()}
        >
          <Icon size={16} aria-hidden="true" />
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <strong>{t(locale, keys.label)}</strong> — {t(locale, keys.description)}
      </TooltipContent>
    </Tooltip>
  );
}

function StatusIcon({ status }: { status: string }) {
  const locale = useLocale();
  const Icon =
    status === "new" ? FilePlus : status === "deleted" ? FileMinus : FileCheck;
  const statusLabel =
    status === "new"
      ? t(locale, "newFile")
      : status === "deleted"
        ? t(locale, "deletedFile")
        : t(locale, "updatedFile");
  return (
    <span
      className={`file-status-icon ${status}`}
      title={statusLabel}
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

function CategoryIcon({ name, title }: { name: string; title?: string }) {
  const locale = useLocale();
  const Icon = categoryIcons[name] ?? Tag;
  return (
    <span
      className="category-icon"
      title={title ? categoryName(locale, title) : undefined}
      aria-hidden="true"
    >
      <Icon size={18} />
    </span>
  );
}

type CategoryLike = Pick<CategoryView, "name" | "icon">;

function CategoryBadge({ category }: { category: CategoryLike }) {
  const locale = useLocale();
  return (
    <span className="category-badge">
      <CategoryIcon name={category.icon} />
      <span className="category-badge-label">
        {categoryName(locale, category.name)}
      </span>
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

function FileHeader({
  file,
  category,
}: {
  file: FileLike;
  category?: CategoryLike;
}) {
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
        {category && <CategoryBadge category={category} />}
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

export function highlightDiffText(
  item: DiffItem,
  path: string,
  locale: Locale = detectLocale(),
): string {
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
  const omitted = Math.max(0, source.length - maxRenderedLineLength);
  if (omitted > 0)
    source = `${source.slice(0, maxRenderedLineLength)} … ${t(locale, "omittedCharacters", { count: omitted })}`;
  const language = languageForPath(path);
  let highlighted = escapeHTML(source);
  if (
    omitted === 0 &&
    source.length <= maxHighlightedLineLength &&
    language &&
    hljs.getLanguage(language)
  ) {
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
  const diffMode = useContext(DiffModeContext);
  const locale = useLocale();
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
          {t(locale, direction === "up" ? "expandAbove" : "expandBelow", {
            count: item.count ?? 0,
          })}
        </button>
      );
    }
    const rowClass = item.class ?? "ctx";
    const highlighted = highlightDiffText(item, path, locale);
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
        {diffMode === "unified" ? (
          <>
            <span className="line-number unified-cell">{unifiedNumber}</span>
            <span
              className="line-code unified-cell"
              dangerouslySetInnerHTML={markup(highlighted)}
            />
          </>
        ) : oldNumber === "" && newNumber === "" ? (
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

function FileDiff({ items, path }: { items: DiffItem[]; path: string }) {
  return (
    <pre className="file-diff">
      <DiffItems items={items} path={path} />
    </pre>
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
  const locale = useLocale();
  return questions.mode === "interactive" && questions.active ? (
    <button
      className="ask-button"
      type="button"
      onClick={() => questions.compose(anchor)}
    >
      {t(locale, "ask")}
    </button>
  ) : null;
}

function DisclosureActions({ selector }: { selector: string }) {
  const locale = useLocale();
  const setOpen = (event: React.MouseEvent, open: boolean) => {
    event.preventDefault();
    event.stopPropagation();
    event.currentTarget
      .closest("details")
      ?.querySelectorAll<HTMLDetailsElement>(selector)
      .forEach((details) => (details.open = open));
  };
  return (
    <span className="disclosure-actions">
      <button type="button" onClick={(event) => setOpen(event, true)}>
        {t(locale, "openAll")}
      </button>
      <button type="button" onClick={(event) => setOpen(event, false)}>
        {t(locale, "closeAll")}
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
  const locale = useLocale();
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
            <span
              className="qa-thread-title"
              title={list(thread.turns)[0]?.question ?? t(locale, "question")}
            >
              {t(locale, "questionLabel")}.{" "}
              {list(thread.turns)[0]?.question ?? t(locale, "question")}
            </span>
          </summary>
          {list(thread.turns).map((turn, index) => (
            <div className="qa-turn" key={turn.id}>
              {index > 0 && (
                <>
                  <strong className="qa-label qa-question">
                    {t(locale, "questionLabel")}
                  </strong>
                  <Markdown source={turn.question} />
                </>
              )}
              {turn.answer ? (
                <>
                  <strong className="qa-label qa-answer">
                    {t(locale, "answerLabel")}
                  </strong>
                  <Markdown source={turn.answer} />
                </>
              ) : (
                <>
                  <span />
                  <small>
                    {turn.status === "pending" ||
                    turn.status === "claimed" ||
                    turn.status === "answered"
                      ? t(locale, turn.status)
                      : turn.status}
                  </small>
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
              {t(locale, "askFollowUp")}
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
              {t(locale, "cancel")}
            </button>
            <button type="submit" disabled={!questions.draft.trim()}>
              {t(locale, "ask")}
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
      <Markdown inline source={fragment.description} />
      <Ask anchor={anchor} questions={questions} />
    </div>
  );
}

function GuidedFragment({
  fragment,
  category,
  groupID,
  questions,
}: {
  fragment: FragmentView;
  category?: CategoryLike;
  groupID: string;
  questions: Questions;
}) {
  const locale = useLocale();
  const anchor: Anchor = {
    type: "fragment",
    group_id: groupID,
    fragment_id: fragment.id,
  };
  const [reviewed, setReviewed] = useState(false);
  const detailsRef = useRef<HTMLDetailsElement>(null);
  const diff = [
    ...list(fragment.upper_context),
    ...list(fragment.hunk),
    ...list(fragment.lower_context),
  ];
  return (
    <details
      ref={detailsRef}
      id={`guided-${groupID}-${fragment.id}`}
      className={`guided-file diff-view ${reviewed ? "is-reviewed" : ""}`}
      data-group-id={groupID}
      data-file-path={fragment.path}
    >
      <summary>
        <DisclosureIcon />
        <FileHeader file={fragment} category={category} />
        <button
          className="file-review-toggle"
          type="button"
          aria-pressed={reviewed}
          aria-label={t(locale, reviewed ? "markUnreviewed" : "markReviewed")}
          onClick={(event) => {
            event.preventDefault();
            setReviewed(!reviewed);
            if (!reviewed && detailsRef.current)
              detailsRef.current.open = false;
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
      <FileDiff items={diff} path={fragment.path} />
      <QuestionPanel anchor={anchor} questions={questions} />
    </details>
  );
}

function GuidedGroup({
  group,
  number,
  questions,
  openSteps,
  setStepOpen,
}: {
  group: GroupView;
  number: number;
  questions: Questions;
  openSteps: ReadonlySet<string>;
  setStepOpen: (key: string, open: boolean) => void;
}) {
  const locale = useLocale();
  const groupAnchor: Anchor = { type: "group", group_id: group.id };
  const fragments = new Map(
    list(group.categories).flatMap((category) =>
      list(category.files).flatMap((file) =>
        list(file.fragments).map(
          (fragment) => [fragment.id, fragment] as const,
        ),
      ),
    ),
  );
  const categoriesByPath = new Map(
    list(group.categories).flatMap((category) =>
      list(category.files).map((file) => [file.path, category] as const),
    ),
  );
  return (
    <details
      id={`guided-${group.anchor_id}`}
      className="group guided-group"
      open
    >
      <summary>
        <DisclosureIcon />
        <h2>
          {t(locale, "groupNumber", { number })} {group.title}
        </h2>
        <Importance value={group.importance} />
        <div className="summary-preview">
          <Markdown source={group.summary} />
        </div>
        <span className="count">
          {t(locale, "stepsFragments", {
            steps: list(group.steps).length,
            stepLabel: pluralTerm(list(group.steps).length, "Step"),
            fragments: group.fragment_count,
            fragmentLabel: pluralTerm(group.fragment_count, "Fragment"),
          })}
        </span>
        <DisclosureActions selector=".review-step,.guided-file" />
      </summary>
      <Ask anchor={groupAnchor} questions={questions} />
      <QuestionPanel anchor={groupAnchor} questions={questions} />
      {list(group.steps).map((step) => {
        const stepKey = `${group.id}\0${step.id}`;
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
            open={openSteps.has(stepKey)}
            onToggle={(event) => setStepOpen(stepKey, event.currentTarget.open)}
          >
            <summary>
              <DisclosureIcon />
              <h3>
                {t(locale, "stepNumber", { number: step.number })} {step.title}
              </h3>
              <div className="summary-preview">
                <Markdown source={step.summary} />
              </div>
              <Ask anchor={anchor} questions={questions} />
            </summary>
            <QuestionPanel anchor={anchor} questions={questions} />
            {list(step.fragment_ids).map((fragmentID) => {
              const fragment = fragments.get(fragmentID);
              return fragment ? (
                <GuidedFragment
                  fragment={fragment}
                  category={categoriesByPath.get(fragment.path)}
                  groupID={group.id}
                  questions={questions}
                  key={fragment.id}
                />
              ) : null;
            })}
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
  const locale = useLocale();
  const [reviewed, setReviewed] = useState(false);
  const body = [
    ...list(file.fragments).flatMap((fragment) => [
      ...list(fragment.upper_context),
      ...list(fragment.hunk),
      ...list(fragment.lower_context),
    ]),
  ];
  return (
    <details
      id={file.anchor_id}
      className={`file main-file diff-view ${reviewed ? "is-reviewed" : ""}`}
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
          aria-label={t(locale, reviewed ? "markUnreviewed" : "markReviewed")}
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
      <FileDiff items={body} path={file.path} />
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
  const locale = useLocale();
  return (
    <details className="category" open>
      <summary>
        <DisclosureIcon />
        <CategoryIcon name={category.icon} />
        <strong>{categoryName(locale, category.name)}</strong>
        <span className="category-stats">
          {category.added
            ? `${t(locale, "added", { count: category.added })} `
            : ""}
          {category.updated
            ? `${t(locale, "updated", { count: category.updated })} `
            : ""}
          {category.deleted
            ? t(locale, "deleted", { count: category.deleted })
            : ""}
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
  number,
  questions,
}: {
  group: GroupView;
  number: number;
  questions: Questions;
}) {
  const locale = useLocale();
  const files = list(group.categories).flatMap((category) =>
    list(category.files),
  );
  return (
    <details id={group.anchor_id} className="group files-group" open>
      <summary>
        <DisclosureIcon />
        <h2>
          {t(locale, "groupNumber", { number })} {group.title}
        </h2>
        <Importance value={group.importance} />
        <div className="summary-preview">
          <Markdown source={group.summary} />
        </div>
        <span className="count">
          {t(locale, "filesFragments", {
            files: files.length,
            fileLabel: pluralTerm(files.length, "File"),
            fragments: group.fragment_count,
            fragmentLabel: pluralTerm(group.fragment_count, "Fragment"),
          })}
        </span>
        <DisclosureActions selector=".category,.file" />
      </summary>
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
  category,
  groupID,
  activeKey,
}: {
  file: FileView;
  category: CategoryLike;
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
      <CategoryIcon name={category.icon} title={category.name} />
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
  categoriesByPath,
  groupID,
  activeKey,
}: {
  directory: FileTreeNode;
  categoriesByPath: ReadonlyMap<string, CategoryLike>;
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
            categoriesByPath={categoriesByPath}
            groupID={groupID}
            activeKey={activeKey}
            key={child.name}
          />
        ))}
        {directory.files.map((file) => (
          <GroupFileLink
            file={file}
            category={
              categoriesByPath.get(file.path) ?? {
                name: "unknown",
                icon: "unknown",
              }
            }
            groupID={groupID}
            activeKey={activeKey}
            key={file.path}
          />
        ))}
      </div>
    </details>
  );
}

function GuidedSidebarGroup({
  group,
  number,
  activeKey,
  openSteps,
  setStepOpen,
}: {
  group: GroupView;
  number: number;
  activeKey: string;
  openSteps: ReadonlySet<string>;
  setStepOpen: (key: string, open: boolean) => void;
}) {
  const locale = useLocale();
  const fragments = new Map(
    list(group.categories).flatMap((category) =>
      list(category.files).flatMap((file) =>
        list(file.fragments).map(
          (fragment) => [fragment.id, { fragment, category }] as const,
        ),
      ),
    ),
  );
  return (
    <details className="nav-group" open>
      <summary>
        <DisclosureIcon />
        <span>
          {t(locale, "groupNumber", { number })} {group.title}
        </span>
        <Importance value={group.importance} />
        <small>{list(group.steps).length}</small>
      </summary>
      <div className="nav-group-files">
        {list(group.steps).map((step) => (
          <details
            className="nav-step"
            key={step.id}
            open={openSteps.has(`${group.id}\0${step.id}`)}
            onToggle={(event) =>
              setStepOpen(`${group.id}\0${step.id}`, event.currentTarget.open)
            }
          >
            <summary>
              <DisclosureIcon />
              <span>
                {t(locale, "stepNumber", { number: step.number })} {step.title}
              </span>
              <small>{list(step.fragment_ids).length}</small>
            </summary>
            <div className="nav-children">
              {list(step.fragment_ids).map((fragmentID) => {
                const entry = fragments.get(fragmentID);
                if (!entry) return null;
                const { fragment, category } = entry;
                return (
                  <button
                    className={`nav-link nav-group-file ${activeKey === `${group.id}\0${fragment.path}` ? "is-active" : ""}`}
                    type="button"
                    onClick={() =>
                      navigate(`guided-${group.id}-${fragment.id}`)
                    }
                    key={fragment.id}
                  >
                    <StatusIcon status={fragment.status} />
                    <CategoryIcon name={category.icon} title={category.name} />
                    <span className="nav-file-path">
                      <span className="nav-file-directory">
                        {fragment.directory}
                      </span>
                      <span className="nav-file-name">{fragment.name}</span>
                    </span>
                    <Level value={fragment.review_level} />
                  </button>
                );
              })}
            </div>
          </details>
        ))}
      </div>
    </details>
  );
}

function Sidebar({
  bootstrap,
  reviewMode,
  openSteps,
  setStepOpen,
}: {
  bootstrap: Bootstrap;
  reviewMode: "guided" | "files";
  openSteps: ReadonlySet<string>;
  setStepOpen: (key: string, open: boolean) => void;
}) {
  const locale = useLocale();
  const [activeKey, setActiveKey] = useState("");
  const [width, setWidth] = useState(() => {
    try {
      return Number(localStorage.getItem("semdiff-sidebar-width")) || 360;
    } catch {
      return 360;
    }
  });
  const [resizing, setResizing] = useState(false);
  useEffect(() => {
    let observer: IntersectionObserver | undefined;
    const observe = () => {
      observer?.disconnect();
      const bottomMargin = Math.max(0, window.innerHeight - 81);
      observer = new IntersectionObserver(
        (entries) => {
          const file = entries
            .filter((entry) => entry.isIntersecting)
            .sort(
              (left, right) =>
                Math.abs(left.boundingClientRect.top - 80) -
                Math.abs(right.boundingClientRect.top - 80),
            )[0]?.target as HTMLElement | undefined;
          if (file)
            setActiveKey(`${file.dataset.groupId}\0${file.dataset.filePath}`);
        },
        { rootMargin: `-80px 0px -${bottomMargin}px 0px` },
      );
      document
        .querySelectorAll<HTMLElement>(".main-file,.guided-file[data-group-id]")
        .forEach((file) => observer?.observe(file));
    };
    observe();
    window.addEventListener("resize", observe);
    return () => {
      window.removeEventListener("resize", observe);
      observer?.disconnect();
    };
  }, [reviewMode]);
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
        {list(bootstrap.page.groups).map((group, index) => {
          if (reviewMode === "guided") {
            return (
              <GuidedSidebarGroup
                group={group}
                number={index + 1}
                activeKey={activeKey}
                openSteps={openSteps}
                setStepOpen={setStepOpen}
                key={group.id}
              />
            );
          }
          const files = list(group.categories).flatMap((category) =>
            list(category.files),
          );
          const categoriesByPath = new Map(
            list(group.categories).flatMap((category) =>
              list(category.files).map(
                (file) => [file.path, category] as const,
              ),
            ),
          );
          const tree = buildFileTree(files);
          return (
            <details className="nav-group" key={group.id} open>
              <summary>
                <DisclosureIcon />
                <span>
                  {t(locale, "groupNumber", { number: index + 1 })}{" "}
                  {group.title}
                </span>
                <Importance value={group.importance} />
                <small>{files.length}</small>
              </summary>
              <div className="nav-group-files">
                {tree.directories.map((directory) => (
                  <GroupDirectory
                    directory={directory}
                    categoriesByPath={categoriesByPath}
                    groupID={group.id}
                    activeKey={activeKey}
                    key={directory.name}
                  />
                ))}
                {tree.files.map((file) => (
                  <GroupFileLink
                    file={file}
                    category={
                      categoriesByPath.get(file.path) ?? {
                        name: "unknown",
                        icon: "unknown",
                      }
                    }
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
  const locale = useLocale();
  const drift = bootstrap.page.drift;
  if (!drift) return null;
  return (
    <section className="review-drift">
      <strong>
        {t(locale, "reviewBehind", { count: list(drift.commits).length })}
      </strong>
      <p>
        {t(locale, "groupsCover")}{" "}
        <code>
          {bootstrap.page.base_sha}...{bootstrap.page.head_sha}
        </code>
        . {t(locale, "currentRange")}{" "}
        <code>
          {drift.current_base_sha}...{drift.current_head_sha}
        </code>
        .
      </p>
      <details>
        <summary>
          <DisclosureIcon />
          {t(locale, "changesSinceReview")}
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

export function expandContext(
  button: HTMLButtonElement,
  container: Element,
  locale: Locale = detectLocale(),
): void {
  const hidden = Array.from(
    container.querySelectorAll<HTMLElement>(".context-hidden[hidden]"),
  );
  const direction = button.dataset.direction === "up" ? "up" : "down";
  const revealed = direction === "up" ? hidden.slice(-10) : hidden.slice(0, 10);
  revealed.forEach((line) => {
    line.hidden = false;
  });
  const remaining = container.querySelectorAll(
    ".context-hidden[hidden]",
  ).length;
  if (remaining === 0) {
    container
      .querySelectorAll(".expand-lines")
      .forEach((item) => item.remove());
    return;
  }
  if (direction === "up") {
    revealed[0]?.before(button);
  } else {
    revealed.at(-1)?.after(button);
  }
  container
    .querySelectorAll<HTMLButtonElement>(".expand-lines")
    .forEach((item) => {
      const itemDirection = item.dataset.direction === "up" ? "up" : "down";
      item.textContent = t(
        locale,
        itemDirection === "up" ? "expandAbove" : "expandBelow",
        { count: remaining },
      );
    });
}

const elementHeight = (element: Element | null, fallback: number) =>
  Math.max(fallback, Math.ceil(element?.getBoundingClientRect().height ?? 0));

export function syncStickyOffsets(shell: HTMLElement): void {
  shell.style.setProperty(
    "--page-header-height",
    `${elementHeight(shell.querySelector(":scope > .page-header"), 52)}px`,
  );
  shell.querySelectorAll<HTMLElement>(".group").forEach((group) => {
    group.style.setProperty(
      "--group-header-height",
      `${elementHeight(group.querySelector(":scope > summary"), 44)}px`,
    );
  });
  shell.querySelectorAll<HTMLElement>(".review-step").forEach((step) => {
    step.style.setProperty(
      "--step-header-height",
      `${elementHeight(step.querySelector(":scope > summary"), 40)}px`,
    );
  });
  shell.querySelectorAll<HTMLElement>(".category").forEach((category) => {
    category.style.setProperty(
      "--category-header-height",
      `${elementHeight(category.querySelector(":scope > summary"), 40)}px`,
    );
  });
}

export function App({ bootstrap }: { bootstrap: Bootstrap }) {
  const [locale, setLocale] = useState<Locale>(() => detectLocale());
  const [reviewMode, setReviewMode] = useState<"guided" | "files">("guided");
  const [diffMode, setDiffMode] = useState<"unified" | "split">("unified");
  const [openSteps, setOpenSteps] = useState<Set<string>>(() => new Set());
  const questions = useQuestions(bootstrap);
  useEffect(() => {
    const updateLocale = () => setLocale(detectLocale());
    window.addEventListener("languagechange", updateLocale);
    document.documentElement.lang = locale;
    document.title = t(locale, "semanticChanges");
    return () => window.removeEventListener("languagechange", updateLocale);
  }, [locale]);
  const setStepOpen = (key: string, open: boolean) => {
    setOpenSteps((current) => {
      if (current.has(key) === open) return current;
      const next = new Set(current);
      if (open) next.add(key);
      else next.delete(key);
      return next;
    });
  };
  useEffect(() => {
    document.body.dataset.view = diffMode;
  }, [diffMode]);
  useEffect(() => {
    if (location.hash)
      requestAnimationFrame(() => navigate(location.hash.slice(1)));
  }, [reviewMode]);
  useEffect(() => {
    const shell = document.querySelector<HTMLElement>(".viewer-shell");
    if (!shell) return;
    const sync = () => syncStickyOffsets(shell);
    const observer = new ResizeObserver(sync);
    sync();
    shell
      .querySelectorAll<HTMLElement>(
        ":scope > .page-header,.group > summary,.review-step > summary,.category > summary",
      )
      .forEach((element) => observer.observe(element));
    return () => observer.disconnect();
  }, [reviewMode]);
  useEffect(() => {
    const expand = (event: MouseEvent) => {
      const button = (
        event.target as Element | null
      )?.closest<HTMLButtonElement>(".expand-lines");
      const container = button?.closest(".context-expand");
      if (!button || !container) return;
      expandContext(button, container, locale);
    };
    document.addEventListener("click", expand);
    return () => document.removeEventListener("click", expand);
  }, [locale]);
  return (
    <LocaleContext.Provider value={locale}>
      <TooltipProvider delayDuration={300}>
        <DiffModeContext.Provider value={diffMode}>
          <div className="viewer-shell">
            <header className="page-header">
              <h1>{t(locale, "semanticChanges")}</h1>
              <code className="revision-range">
                {bootstrap.page.base_sha} → {bootstrap.page.head_sha}
              </code>
              <div className="stats">
                {t(locale, "stats", {
                  groups: list(bootstrap.page.groups).length,
                  groupLabel: pluralTerm(
                    list(bootstrap.page.groups).length,
                    "Group",
                  ),
                  files: bootstrap.page.file_count,
                  fileLabel: pluralTerm(bootstrap.page.file_count, "File"),
                  fragments: bootstrap.page.fragment_count,
                  fragmentLabel: pluralTerm(
                    bootstrap.page.fragment_count,
                    "Fragment",
                  ),
                })}
              </div>
              <div className="toolbar">
                <div>
                  {(["guided", "files"] as const).map((mode) => (
                    <button
                      key={mode}
                      aria-pressed={reviewMode === mode}
                      onClick={() => setReviewMode(mode)}
                    >
                      {t(locale, mode)}
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
                      {t(locale, mode)}
                    </button>
                  ))}
                </div>
              </div>
              {questions.mode === "interactive" && (
                <button
                  className="answer-mode"
                  type="button"
                  disabled={!questions.active}
                  onClick={questions.stop}
                >
                  {t(
                    locale,
                    questions.active ? "endAnswerMode" : "answerModeStopped",
                  )}
                </button>
              )}
            </header>
            <div className="app-shell">
              <Sidebar
                bootstrap={bootstrap}
                reviewMode={reviewMode}
                openSteps={openSteps}
                setStepOpen={setStepOpen}
              />
              <main className="wrap">
                <Drift bootstrap={bootstrap} />
                {list(bootstrap.page.groups).map((group, index) =>
                  reviewMode === "guided" ? (
                    <GuidedGroup
                      group={group}
                      number={index + 1}
                      questions={questions}
                      openSteps={openSteps}
                      setStepOpen={setStepOpen}
                      key={group.id}
                    />
                  ) : (
                    <FilesGroup
                      group={group}
                      number={index + 1}
                      questions={questions}
                      key={group.id}
                    />
                  ),
                )}
              </main>
            </div>
          </div>
        </DiffModeContext.Provider>
      </TooltipProvider>
    </LocaleContext.Provider>
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
    const locale = detectLocale();
    document.documentElement.lang = locale;
    document.title = t(locale, "semanticChanges");
    createRoot(root).render(
      <main className="bootstrap-error">
        <h1>{t(locale, "semanticChanges")}</h1>
        <p>{t(locale, "invalidViewerData", { error: String(error) })}</p>
      </main>,
    );
  }
}
