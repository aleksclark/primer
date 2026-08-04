import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * System C input: square, raised surface, 1px rule.
 * Focus = accent border + 1px accent outline at 2px offset.
 * Error state via aria-invalid → attention border.
 */
function Input({ className, type, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      type={type}
      className={cn(
        [
          "flex h-10 w-full rounded-none border border-input bg-surface-raised",
          "px-3.5 py-3 text-sm text-foreground",
          "placeholder:text-muted-foreground",
          "transition-colors",
          "focus-visible:border-primary focus-visible:outline focus-visible:outline-[length:var(--primer-focus-width)]",
          "focus-visible:outline-offset-[var(--primer-focus-offset)] focus-visible:outline-primary",
          "aria-invalid:border-attention aria-invalid:text-foreground",
          "disabled:cursor-not-allowed disabled:border-border disabled:text-rule-strong",
          "file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground",
        ].join(" "),
        className,
      )}
      {...props}
    />
  );
}

export { Input };
