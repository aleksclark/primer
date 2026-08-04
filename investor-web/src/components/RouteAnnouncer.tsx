import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";

const titles: Record<string, string> = {
  "/": "Primer investor pitch",
  "/demo": "Product demo — Primer",
  "/market": "Market model — Primer",
  "/evidence": "Evidence register — Primer",
  "/schools": "Schools path — Primer",
  "/company": "Company — Primer",
  "/diligence": "Diligence — Primer",
};

/** Accessible route change announcements for screen readers. */
export function RouteAnnouncer() {
  const location = useLocation();
  const [message, setMessage] = useState("");

  useEffect(() => {
    const title = titles[location.pathname] ?? "Primer";
    document.title = title;
    setMessage(title);
  }, [location.pathname]);

  return (
    <div className="sr-only" aria-live="polite" aria-atomic="true" role="status">
      {message}
    </div>
  );
}
