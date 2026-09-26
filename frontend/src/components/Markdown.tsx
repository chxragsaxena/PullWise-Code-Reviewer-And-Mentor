import ReactMarkdown from "react-markdown";
import "./Markdown.css";

/** Renders lesson Markdown. Raw HTML in the source is not rendered. */
export function Markdown({ children }: { children: string }) {
  return (
    <div className="prose">
      <ReactMarkdown>{children}</ReactMarkdown>
    </div>
  );
}
