import type { Source } from "@/data/types";
import { analytics } from "@/lib/analytics";
import { cn } from "@/lib/cn";

interface SourceLinkProps {
  source: Source;
  className?: string;
}

/** Citation link adjacent to a claim. Opens in a new tab. */
export function SourceLink({ source, className }: SourceLinkProps) {
  return (
    <a
      className={cn("source-link", className)}
      href={source.url}
      target="_blank"
      rel="noopener noreferrer"
      title={`${source.title} — ${source.organization}`}
      onClick={() => analytics.sourceClick(source.id)}
    >
      <span aria-hidden="true">↗</span>
      <span>{source.organization || source.title}</span>
    </a>
  );
}

interface SourceLinkListProps {
  sources: Source[];
  className?: string;
}

export function SourceLinkList({ sources, className }: SourceLinkListProps) {
  if (sources.length === 0) return null;
  return (
    <div className={cn("source-link-list", className)} role="list">
      {sources.map((source) => (
        <span key={source.id} role="listitem">
          <SourceLink source={source} />
        </span>
      ))}
    </div>
  );
}
