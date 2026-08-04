import { PrimaryButton, SystemLabel, TextLink } from "@/components";

interface PlaceholderPageProps {
  title: string;
  description: string;
  eyebrow?: string;
}

/** Diligence/demo route shell until Phase 4 content lands. */
export function PlaceholderPage({
  title,
  description,
  eyebrow = "Diligence route",
}: PlaceholderPageProps) {
  return (
    <div className="route-page">
      <div className="site-container">
        <header className="route-page__header">
          <SystemLabel tone="accent">{eyebrow}</SystemLabel>
          <h1 className="type-h1" style={{ margin: 0 }}>
            {title}
          </h1>
          <p className="type-body prose-measure text-muted" style={{ margin: 0 }}>
            {description}
          </p>
        </header>
        <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--primer-space-4)" }}>
          <PrimaryButton href="/">Back to pitch</PrimaryButton>
          <TextLink href="/#contact">Discuss the round</TextLink>
        </div>
      </div>
    </div>
  );
}
