import { ChatFilesContext, ChatFileLink, workspaceFilePath } from "../features/split/chatFiles";
import { memo, useContext } from "react";
import ReactMarkdown from "react-markdown";
import rehypeKatex from "rehype-katex";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";
import type { Paragraph, Root } from "mdast";
import type {} from "mdast-util-math";
import { remarkBackslashMath } from "../features/split/mathDelimiters";
import "katex/dist/katex.min.css";
import "../styles/math.css";

/**
 * remark-math parses a one-line whole-paragraph `$$...$$` as inline math, so
 * without help a standalone display formula would typeset inline. Promote
 * exactly that case to display math by retagging the parsed node; `$$` used
 * mid-sentence stays inline. This reuses the parsed tree and the delimiter
 * width recorded in node positions - it is not a separate parser.
 */
function remarkStandaloneDisplayMath(): (tree: Root) => void {
  const walk = (node: { readonly type: string; readonly children?: readonly unknown[] }): void => {
    for (const child of node.children ?? []) {
      walk(child as { readonly type: string; readonly children?: readonly unknown[] });
    }
    if (node.type !== "paragraph") return;
    const children = (node as unknown as Paragraph).children;
    if (children.length !== 1) return;
    const child = children[0];
    if (child?.type !== "inlineMath" || child.position === undefined) return;
    if ((child.data as { readonly backslashDelimiter?: string } | undefined)?.backslashDelimiter === "(") return;
    const { start, end } = child.position;
    if (start.offset === undefined || end.offset === undefined) return;
    // `$$` fences are two columns wider per side than `$` fences.
    if (end.offset - start.offset - child.value.length !== 4) return;
    child.data = {
      ...child.data,
      hProperties: {
        ...(child.data?.hProperties as Record<string, unknown> | undefined),
        className: ["language-math", "math-display"],
      },
    };
  };
  return (tree) => walk(tree);
}

function remarkFileCode(): (tree: Root) => void {
  return tree => {
    const walk = (node: { type: string; children?: unknown[] | undefined; data?: { hProperties?: Record<string, unknown> | undefined } | undefined }, insideLink = false): void => {
      if (node.type === "inlineCode" && !insideLink) node.data = { ...node.data, hProperties: { ...node.data?.hProperties, "data-file-code": true } };
      for (const child of node.children ?? []) walk(child as Parameters<typeof walk>[0], insideLink || node.type === "link");
    };
    walk(tree);
  };
}

// One Markdown seam for finalized blocks and the streaming buffer alike, so
// both render math identically. remark-math only pairs balanced $...$/$$...$$
// delimiters, so a half-typed formula mid-stream stays literal text; a
// malformed but delimited one is rendered back as source inline by KaTeX
// (throwOnError: false) instead of throwing or blanking the transcript.
// Models also emit \[...\] / \(...\) delimiters, which CommonMark consumes as
// bracket escapes. After remark-math has registered dollar-math parsing,
// remarkBackslashMath recovers balanced pairs from text-node source positions
// and creates math nodes directly; the display promotion then handles both
// existing standalone $$ nodes and backslash-bracket display nodes.
export const Markdown = memo(({ text, imageUrl }: { readonly text: string; readonly imageUrl?: (src: string) => string }) => {
 const files = useContext(ChatFilesContext);
 return <ReactMarkdown
    remarkPlugins={[remarkGfm, remarkMath, remarkBackslashMath, remarkStandaloneDisplayMath, remarkFileCode]}
    components={{
      a: ({ href, children }) => {
        let decoded = href ?? "";
        try { decoded = decodeURIComponent(decoded); } catch { /* Invalid URL remains a regular link. */ }
        const path = files ? workspaceFilePath(decoded, files.root, false) : null;
        return path ? <ChatFileLink path={path}>{children}</ChatFileLink> : <a href={href}>{children}</a>;
      },
      code: ({ children, className, node, ...props }) => {
        // The remark marker excludes fenced code and code nested inside links.
        const value = String(children);
        const path = files && node?.properties["data-file-code"] && !className && !value.includes("\n") ? workspaceFilePath(value, files.root) : null;
        return path ? <ChatFileLink path={path}>{children}</ChatFileLink> : <code className={className} {...props}>{children}</code>;
      },
      ...(imageUrl ? {
      img: ({ src, alt }) => <img src={imageUrl(src ?? "")} alt={alt ?? ""} />,
      table: ({ children }) => <div className="th-viewer-table"><table>{children}</table></div>,
    } : {}),
    }}
    rehypePlugins={[[rehypeKatex, { throwOnError: false, strict: false }]]}
  >
    {text}
  </ReactMarkdown>;
});

