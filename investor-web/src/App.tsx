import { investorData } from "./data";

/**
 * Minimal scaffold so Phase 1 can mount presentation components.
 * Phase 0 freezes structured claims only — no pitch UI yet.
 */
export function App() {
  return (
    <main style={{ fontFamily: "system-ui, sans-serif", padding: "2rem", maxWidth: 720 }}>
      <h1>Primer investor data package</h1>
      <p>
        Phase 0 content contract is loaded. {investorData.sections.length} sections,{" "}
        {investorData.marketLayers.length} market layers, {investorData.sources.length}{" "}
        sources.
      </p>
      <p style={{ color: "#555" }}>
        Presentation components land in later phases. Run{" "}
        <code>npm test</code> to validate claims.
      </p>
    </main>
  );
}
