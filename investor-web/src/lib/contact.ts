/**
 * Contact CTA helpers. Prefer env override for real deploys.
 * Placeholder local address is intentional until a public inbox is chosen.
 */

const FALLBACK_EMAIL = "aleks@primer.local";

export function contactEmail(): string {
  const fromEnv = (import.meta.env.VITE_CONTACT_EMAIL as string | undefined)?.trim();
  return fromEnv && fromEnv.length > 0 ? fromEnv : FALLBACK_EMAIL;
}

export function contactMailto(subject = "Primer pre-seed conversation"): string {
  return `mailto:${contactEmail()}?subject=${encodeURIComponent(subject)}`;
}

export function contactDisplayEmail(): string {
  return contactEmail();
}
