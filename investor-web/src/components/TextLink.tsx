import type { AnchorHTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/cn";

interface TextLinkProps extends AnchorHTMLAttributes<HTMLAnchorElement> {
  children: ReactNode;
  external?: boolean;
}

/** Quiet mono text link for secondary navigation and citations. */
export function TextLink({
  children,
  className,
  external,
  rel,
  target,
  ...rest
}: TextLinkProps) {
  return (
    <a
      className={cn("text-link", className)}
      target={external ? "_blank" : target}
      rel={external ? "noopener noreferrer" : rel}
      {...rest}
    >
      {children}
      {external ? <span aria-hidden="true"> ↗</span> : null}
    </a>
  );
}
