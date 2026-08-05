import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

interface CaveatNoteProps {
  children: ReactNode;
  label?: string;
  /** Stronger treatment for publication blockers. */
  blocker?: boolean;
  className?: string;
}

/** Inline caveat or publication blocker next to a claim. */
export function CaveatNote({
  children,
  label = "Caveat",
  blocker = false,
  className,
}: CaveatNoteProps) {
  return (
    <div
      className={cn("caveat-note", blocker && "caveat-note--blocker", className)}
      role="note"
    >
      <span className="caveat-note__label">{blocker ? "Blocker" : label}</span>
      <span>{children}</span>
    </div>
  );
}
