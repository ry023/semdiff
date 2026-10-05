import { createContext, useContext } from "react";

export type Locale = "en" | "ja";

const english = {
  semanticChanges: "Semantic Review",
  invalidViewerData: "Viewer data is invalid: {error}",
  stats: "{groups} groups · {files} files · {fragments} fragments",
  guided: "Guided",
  files: "Files",
  unified: "Unified",
  split: "Split",
  endAnswerMode: "End answer mode",
  answerModeStopped: "Answer mode stopped",
  ask: "Ask",
  askFollowUp: "Ask follow-up",
  cancel: "Cancel",
  markReviewed: "Mark as reviewed",
  markUnreviewed: "Mark as not reviewed",
  groupNumber: "Group {number}.",
  stepNumber: "Step {number}.",
  question: "Question",
  questionLabel: "Q",
  answerLabel: "A",
  openAll: "Open all",
  closeAll: "Close all",
  stepsFragments: "{steps} steps · {fragments} fragments",
  filesFragments: "{files} files · {fragments} fragments",
  added: "{count} added",
  updated: "{count} updated",
  deleted: "{count} deleted",
  newFile: "new file",
  updatedFile: "updated file",
  deletedFile: "deleted file",
  pending: "pending",
  claimed: "in progress",
  answered: "answered",
  categoryImplementation: "implementation",
  categoryTest: "test",
  categoryComponent: "component",
  categoryLogic: "logic",
  categoryConfig: "config",
  categoryDocs: "docs",
  categoryUnknown: "unknown",
  categoryCustom: "custom",
  importanceCore: "core",
  importanceSupporting: "supporting",
  importanceSide: "side",
  importanceCoreDescription: "Defines the PR's purpose or essential behavior.",
  importanceSupportingDescription:
    "Implements, adapts, or verifies the core change.",
  importanceSideDescription:
    "A separate meaningful change, not needed for the core purpose.",
  reviewCareful: "careful",
  reviewNormal: "normal",
  reviewSkim: "skim",
  reviewCarefulDescription: "Read this fragment closely.",
  reviewNormalDescription: "Review this fragment at an ordinary level.",
  reviewSkimDescription: "A quick check is enough for this fragment.",
  expandAbove: "↑ Show {count} lines above",
  expandBelow: "↓ Show {count} lines below",
  omittedCharacters: "[{count} characters omitted]",
  reviewBehind:
    "This semantic review is {count} unreviewed commits behind HEAD.",
  groupsCover: "Groups cover",
  currentRange: "Current range:",
  changesSinceReview: "Changes since this review",
} as const;

export type MessageKey = keyof typeof english;
type Messages = Record<MessageKey, string>;

const japanese: Messages = {
  semanticChanges: "Semantic Review",
  invalidViewerData: "ビューアーデータが不正です: {error}",
  stats: "{groups} グループ · {files} ファイル · {fragments} フラグメント",
  guided: "Guided",
  files: "Files",
  unified: "Unified",
  split: "Split",
  endAnswerMode: "回答モードを終了",
  answerModeStopped: "回答モード終了済み",
  ask: "質問する",
  askFollowUp: "追加で質問する",
  cancel: "キャンセル",
  markReviewed: "確認済みにする",
  markUnreviewed: "未確認に戻す",
  groupNumber: "Group {number}.",
  stepNumber: "Step {number}.",
  question: "質問",
  questionLabel: "質問",
  answerLabel: "回答",
  openAll: "すべて開く",
  closeAll: "すべて閉じる",
  stepsFragments: "{steps} ステップ · {fragments} フラグメント",
  filesFragments: "{files} ファイル · {fragments} フラグメント",
  added: "{count} 追加",
  updated: "{count} 更新",
  deleted: "{count} 削除",
  newFile: "新規ファイル",
  updatedFile: "更新ファイル",
  deletedFile: "削除ファイル",
  pending: "回答待ち",
  claimed: "回答中",
  answered: "回答済み",
  categoryImplementation: "実装",
  categoryTest: "テスト",
  categoryComponent: "コンポーネント",
  categoryLogic: "ロジック",
  categoryConfig: "設定",
  categoryDocs: "ドキュメント",
  categoryUnknown: "その他",
  categoryCustom: "カスタム",
  importanceCore: "Core",
  importanceSupporting: "Supporting",
  importanceSide: "Side",
  importanceCoreDescription: "PR の目的や不可欠な動作を定義します。",
  importanceSupportingDescription: "中核となる変更を実装・調整・検証します。",
  importanceSideDescription: "PR の主目的には必須ではない、独立した変更です。",
  reviewCareful: "Careful",
  reviewNormal: "Normal",
  reviewSkim: "Skim",
  reviewCarefulDescription: "この Fragment は細部まで確認してください。",
  reviewNormalDescription: "この Fragment は通常の詳しさで確認してください。",
  reviewSkimDescription: "この Fragment は概要を確認すれば十分です。",
  expandAbove: "↑ 上に {count} 行表示",
  expandBelow: "↓ 下に {count} 行表示",
  omittedCharacters: "[{count} 文字を省略]",
  reviewBehind:
    "このレビューは HEAD より未確認のコミットが {count} 件遅れています。",
  groupsCover: "レビュー対象の範囲:",
  currentRange: "現在の範囲:",
  changesSinceReview: "このレビュー以降の変更",
};

const catalogs: Record<Locale, Messages> = { en: english, ja: japanese };

export function detectLocale(languages?: readonly string[]): Locale {
  const preferred =
    languages ??
    (typeof navigator === "undefined"
      ? []
      : navigator.languages?.length
        ? navigator.languages
        : [navigator.language]);
  for (const language of preferred) {
    const base = language.toLowerCase().split("-")[0];
    if (base === "ja") return "ja";
    if (base === "en") return "en";
  }
  return "en";
}

export function t(
  locale: Locale,
  key: MessageKey,
  values: Record<string, string | number> = {},
): string {
  return catalogs[locale][key].replace(/\{(\w+)\}/g, (_, name: string) =>
    String(values[name] ?? `{${name}}`),
  );
}

export function categoryName(locale: Locale, name: string): string {
  const keys: Record<string, MessageKey> = {
    implementation: "categoryImplementation",
    test: "categoryTest",
    component: "categoryComponent",
    logic: "categoryLogic",
    config: "categoryConfig",
    docs: "categoryDocs",
    unknown: "categoryUnknown",
    custom: "categoryCustom",
  };
  const key = keys[name];
  return key ? t(locale, key) : name;
}

export const LocaleContext = createContext<Locale>("en");
export const useLocale = () => useContext(LocaleContext);
