import type { HTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/cn";

type SystemLabelTone = "muted" | "accent" | "attention" | "text";

interface SystemLabelProps extends HTMLAttributes<HTMLSpanElement> {
  children: ReactNode;
  tone?: SystemLabelTone;
  as?: "span" | "p" | "div";
}

const toneClass: Record<SystemLabelTone, string> = {
  muted: "",
  accent: "system-label--accent",
  attention: "system-label--attention",
  text: "system-label--text",
};

/** IBM Plex Mono uppercase metadata label. */
export function SystemLabel({
  children,
  tone = "muted",
  as: Tag = "span",
  className,
  ...rest
}: SystemLabelProps) {
  return (
    <Tag className={cn("system-label", toneClass[tone], className)} {...rest}>
      {children}
    </Tag>
  );
}
