import type { AnchorHTMLAttributes, ButtonHTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/cn";

type Variant = "primary" | "secondary" | "quiet";

type CommonProps = {
  children: ReactNode;
  variant?: Variant;
  className?: string;
  disabled?: boolean;
};

type ButtonAsButton = CommonProps &
  Omit<ButtonHTMLAttributes<HTMLButtonElement>, "className" | "children"> & {
    href?: undefined;
  };

type ButtonAsLink = CommonProps &
  Omit<AnchorHTMLAttributes<HTMLAnchorElement>, "className" | "children"> & {
    href: string;
  };

export type PrimaryButtonProps = ButtonAsButton | ButtonAsLink;

const variantClass: Record<Variant, string> = {
  primary: "btn--primary",
  secondary: "btn--secondary",
  quiet: "btn--quiet",
};

/**
 * System C control. Accent fill for primary actions only.
 * Renders as <a> when href is provided, otherwise <button>.
 */
export function PrimaryButton(props: PrimaryButtonProps) {
  const { children, variant = "primary", className, disabled, ...rest } = props;
  const classes = cn("btn", variantClass[variant], className);

  if ("href" in props && props.href) {
    const { href, ...anchorRest } = rest as AnchorHTMLAttributes<HTMLAnchorElement>;
    return (
      <a
        className={classes}
        href={href}
        aria-disabled={disabled || undefined}
        tabIndex={disabled ? -1 : undefined}
        {...anchorRest}
      >
        {children}
      </a>
    );
  }

  const buttonRest = rest as ButtonHTMLAttributes<HTMLButtonElement>;
  return (
    <button type="button" className={classes} disabled={disabled} {...buttonRest}>
      {children}
    </button>
  );
}
