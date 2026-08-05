import { CaveatNote } from "@/components/CaveatNote";
import { SystemLabel } from "@/components/SystemLabel";
import { competitorCategories, type CompetitorSupport } from "@/data";

const CAPABILITY_LABELS: { key: keyof (typeof competitorCategories)[0]["capabilities"]; label: string }[] = [
  { key: "adultDirected", label: "Adult-directed" },
  { key: "longitudinalPlanning", label: "Longitudinal planning" },
  { key: "crossSubjectHabits", label: "Cross-subject habits" },
  { key: "masteryEvidence", label: "Mastery evidence" },
  { key: "projectsAsInstruction", label: "Projects as instruction" },
  { key: "offScreenWork", label: "Off-screen work" },
  { key: "householdFirst", label: "Household-first" },
];

function supportMark(value: CompetitorSupport): string {
  switch (value) {
    case "yes":
      return "Yes";
    case "partial":
      return "Partial";
    case "no":
      return "No";
    default:
      return "Unknown";
  }
}

/**
 * Category matrix — directional synthesis, no unverified universal checkmarks.
 * Desktop table + stacked mobile cards keep identical reading order.
 */
export function CompetitionSection() {
  const rows = competitorCategories;

  return (
    <div className="section-extras">
      <CaveatNote>
        Capability ratings are directional synthesis from official product pages. Primer row
        describes the target operating model — many elements are in development or planned.
      </CaveatNote>

      <div className="matrix-scroll" role="region" aria-label="Competition category matrix">
        <table className="matrix-table">
          <thead>
            <tr>
              <th scope="col">
                <SystemLabel>Category</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Examples</SystemLabel>
              </th>
              <th scope="col">
                <SystemLabel>Customers buy</SystemLabel>
              </th>
              {CAPABILITY_LABELS.map((cap) => (
                <th key={cap.key} scope="col">
                  <SystemLabel>{cap.label}</SystemLabel>
                </th>
              ))}
              <th scope="col">
                <SystemLabel>Primer response</SystemLabel>
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const isPrimer = row.id === "primer";
              return (
                <tr key={row.id} data-primer={isPrimer ? "true" : undefined}>
                  <th scope="row" className="type-small">
                    {row.category}
                  </th>
                  <td className="type-small text-muted">{row.examples.join(" · ")}</td>
                  <td className="type-small">{row.whatCustomersBuy}</td>
                  {CAPABILITY_LABELS.map((cap) => (
                    <td key={cap.key} className="type-small matrix-table__support">
                      <span data-support={row.capabilities[cap.key]}>
                        {supportMark(row.capabilities[cap.key])}
                      </span>
                    </td>
                  ))}
                  <td className="type-small">{row.primerResponse}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <ul className="matrix-stack" aria-label="Competition categories, stacked">
        {rows.map((row) => (
          <li key={row.id} className="matrix-stack__card" data-primer={row.id === "primer" ? "true" : undefined}>
            <SystemLabel tone={row.id === "primer" ? "accent" : "text"}>{row.category}</SystemLabel>
            <p className="type-small text-muted" style={{ margin: 0 }}>
              {row.examples.join(" · ")}
            </p>
            <p className="type-small" style={{ margin: 0 }}>
              <strong>Buy:</strong> {row.whatCustomersBuy}
            </p>
            <ul className="section-list section-list--compact">
              {CAPABILITY_LABELS.map((cap) => (
                <li key={cap.key} className="type-small">
                  {cap.label}: {supportMark(row.capabilities[cap.key])}
                </li>
              ))}
            </ul>
            <p className="type-small" style={{ margin: 0 }}>
              <strong>Primer:</strong> {row.primerResponse}
            </p>
            {row.notes ? <CaveatNote>{row.notes}</CaveatNote> : null}
          </li>
        ))}
      </ul>
    </div>
  );
}
