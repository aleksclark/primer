/** Skip navigation target for keyboard users. */
export function SkipLink({ href = "#main-content", children = "Skip to content" }: {
  href?: string;
  children?: string;
}) {
  return (
    <a className="skip-link type-button" href={href}>
      {children}
    </a>
  );
}
