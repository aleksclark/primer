import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

/**
 * System C buttons: primary / secondary (outline) / quiet (ghost).
 * Monospace uppercase tracked type, zero radius, no shadows.
 */
const buttonVariants = cva(
  [
    "type-button inline-flex items-center justify-center gap-2 whitespace-nowrap",
    "rounded-none transition-colors",
    "focus-visible:outline focus-visible:outline-[length:var(--primer-focus-width)]",
    "focus-visible:outline-offset-[var(--primer-focus-offset)] focus-visible:outline-ring",
    "disabled:pointer-events-none",
    "[&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
  ].join(" "),
  {
    variants: {
      variant: {
        default: [
          "bg-primary text-primary-foreground",
          "hover:bg-accent-hover",
          "disabled:bg-surface-raised disabled:text-rule-strong",
        ].join(" "),
        destructive: [
          "border border-destructive bg-transparent text-destructive",
          "hover:border-destructive hover:bg-transparent hover:text-destructive",
          "disabled:border-border disabled:text-rule-strong",
        ].join(" "),
        outline: [
          "border border-border bg-transparent text-foreground",
          "hover:border-muted-foreground",
          "disabled:border-border disabled:text-rule-strong",
        ].join(" "),
        secondary: [
          "border border-border bg-transparent text-foreground",
          "hover:border-muted-foreground",
          "disabled:border-border disabled:text-rule-strong",
        ].join(" "),
        ghost: [
          "bg-transparent text-muted-foreground",
          "hover:text-foreground hover:underline hover:decoration-foreground hover:underline-offset-4",
          "disabled:text-rule-strong disabled:no-underline",
        ].join(" "),
        link: [
          "bg-transparent text-muted-foreground",
          "hover:text-foreground hover:underline hover:decoration-foreground hover:underline-offset-4",
          "disabled:text-rule-strong",
        ].join(" "),
      },
      size: {
        default: "h-10 px-5 py-3",
        sm: "h-8 px-3 py-2 text-[0.6875rem]",
        lg: "h-11 px-8 py-3",
        icon: "h-9 w-9 p-0",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

function Button({ className, variant, size, asChild = false, ...props }: ButtonProps) {
  const Comp = asChild ? Slot : "button";
  return <Comp className={cn(buttonVariants({ variant, size, className }))} {...props} />;
}

export { Button, buttonVariants };
