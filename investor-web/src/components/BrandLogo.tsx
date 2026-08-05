import { cn } from "@/lib/cn";

interface BrandLogoProps {
  /** "lockup" = wordmark, "mark" = symbol only. */
  variant?: "lockup" | "mark";
  className?: string;
  /** Accessible name; decorative when empty string and aria-hidden. */
  alt?: string;
}

/**
 * Theme-aware Primer logo from design-system assets.
 * Dark and light SVGs swap via CSS on data-theme.
 */
export function BrandLogo({ variant = "lockup", className, alt = "Primer LMS" }: BrandLogoProps) {
  const darkSrc = variant === "mark" ? "/logo/logo-mark.svg" : "/logo/logo.svg";
  const lightSrc = variant === "mark" ? "/logo/logo-mark-light.svg" : "/logo/logo-light.svg";

  return (
    <span className={cn("brand-logo", className)}>
      <img className="logo-dark" src={darkSrc} alt={alt} height={28} />
      <img className="logo-light" src={lightSrc} alt="" aria-hidden="true" height={28} />
    </span>
  );
}
