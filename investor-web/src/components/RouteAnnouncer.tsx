import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import { usePageMeta } from "@/hooks/usePageMeta";
import { analytics } from "@/lib/analytics";
import { metaForPath } from "@/lib/siteMeta";

const DILIGENCE_PREFIXES = ["/demo", "/market", "/evidence", "/schools", "/company", "/diligence"];

/** Accessible route change announcements + document metadata sync. */
export function RouteAnnouncer() {
  const location = useLocation();
  const [message, setMessage] = useState("");
  usePageMeta();

  useEffect(() => {
    const meta = metaForPath(location.pathname);
    setMessage(meta.title);

    if (DILIGENCE_PREFIXES.some((p) => location.pathname === p || location.pathname.startsWith(`${p}/`))) {
      analytics.diligenceRouteView(location.pathname);
    }
  }, [location.pathname]);

  return (
    <div className="sr-only" aria-live="polite" aria-atomic="true" role="status">
      {message}
    </div>
  );
}
