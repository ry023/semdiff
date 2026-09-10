export type ReviewLevel = "careful" | "normal" | "skim" | "";
export type Importance = "core" | "supporting" | "side" | "";

export interface FragmentView {
  id: string;
  path: string;
  description: string;
  description_html: string;
  review_level: ReviewLevel;
  range_label: string;
  directory: string;
  name: string;
  status: string;
  status_icon_html: string;
  additions: number;
  deletions: number;
  diffstat: string[] | null;
  header_html: string;
  hunk_html: string;
  upper_context_html: string;
  lower_context_html: string;
}
export interface FileView {
  path: string;
  anchor_id: string;
  directory: string;
  name: string;
  status: string;
  status_icon_html: string;
  additions: number;
  deletions: number;
  diffstat: string[] | null;
  header_html: string;
  fragments: FragmentView[] | null;
  review_level: ReviewLevel;
}
export interface ReviewStepView {
  id: string;
  title: string;
  summary: string;
  summary_html: string;
  anchor_id: string;
  number: number;
  fragments: FragmentView[] | null;
}
export interface CategoryView {
  name: string;
  icon_html: string;
  icon_class: string;
  standard: boolean;
  files: FileView[] | null;
  added: number;
  updated: number;
  deleted: number;
}
export interface GroupView {
  id: string;
  title: string;
  summary: string;
  summary_html: string;
  importance: Importance;
  anchor_id: string;
  order?: number;
  files: FileView[] | null;
  categories: CategoryView[] | null;
  steps: ReviewStepView[] | null;
  fragment_count: number;
}
export interface SidebarOccurrence {
  group_id: string;
  group_title: string;
  file_anchor_id: string;
  fragment_count: number;
  review_level: ReviewLevel;
}
export interface SidebarFile {
  path: string;
  name: string;
  status_icon_html: string;
  review_level: ReviewLevel;
  occurrences: SidebarOccurrence[] | null;
}
export interface SidebarDirectory {
  name: string;
  file_count: number;
  directories: SidebarDirectory[] | null;
  files: SidebarFile[] | null;
}
export interface Drift {
  current_base_sha: string;
  current_head_sha: string;
  commits: Array<{
    sha: string;
    subject: string;
    files_changed: number;
  }> | null;
  paths: string[] | null;
}
export interface Page {
  base_sha: string;
  head_sha: string;
  drift?: Drift;
  groups: GroupView[] | null;
  sidebar_directories: SidebarDirectory[] | null;
  sidebar_files: SidebarFile[] | null;
  fragment_count: number;
  file_count: number;
}
export interface Anchor {
  type: "group" | "step" | "fragment";
  group_id: string;
  step_id?: string;
  fragment_id?: string;
}
export interface Turn {
  id: string;
  question: string;
  status: string;
  answer?: string;
  question_html: string;
  answer_html?: string;
}
export interface Thread {
  id: string;
  anchor: Anchor;
  turns: Turn[] | null;
}
export interface Bootstrap {
  page: Page;
  capabilities: {
    questions: "disabled" | "readonly" | "interactive";
    api_base?: string;
  };
  threads?: Thread[];
}
