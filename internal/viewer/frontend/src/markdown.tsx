import ReactMarkdown from "react-markdown";
import remarkBreaks from "remark-breaks";
import remarkGfm from "remark-gfm";

export function Markdown({
  source,
  inline = false,
}: {
  source: string;
  inline?: boolean;
}) {
  if (!source) return null;
  return (
    <div className={`markdown${inline ? " markdown-inline" : ""}`}>
      <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]}>
        {source}
      </ReactMarkdown>
    </div>
  );
}
