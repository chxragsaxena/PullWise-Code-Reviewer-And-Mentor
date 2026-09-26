import "./CodeBlock.css";

export function CodeBlock({ caption, children }: { caption: string; children: string }) {
  return (
    <figure className="code-block">
      <figcaption>{caption}</figcaption>
      <pre tabIndex={0}>
        <code>{children}</code>
      </pre>
    </figure>
  );
}
