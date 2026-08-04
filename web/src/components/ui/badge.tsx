import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

/**
 * System C state labels: mono uppercase tracked chips.
 * default = filled accent (e.g. MASTERED)
 * outline = accent border (e.g. IN PROGRESS)
 * secondary = muted rule border (e.g. LOCKED)
 * destructive = attention border (e.g. NEEDS REVIEW)
 */
const badgeVariants = cva(
  "type-label inline-flex items-center rounded-none border px-2.5 py-1.5 transition-colors",
  {
    variants: {
      variant: {
        default: "border-transparent bg-primary text-primary-foreground",
        secondary: "border-border bg-transparent text-muted-foreground",
        destructive: "border-attention bg-transparent text-attention",
        outline: "border-primary bg-transparent text-primary",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
);

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />;
}

export { Badge, badgeVariants };
