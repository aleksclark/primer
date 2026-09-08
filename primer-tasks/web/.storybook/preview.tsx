import type { Preview } from "@storybook/react-vite";
import { useEffect, type ReactNode } from "react";
import "../src/index.css";

function ThemeFrame({ theme, children }: { theme: string; children: ReactNode }) {
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  return <main className="main storybook-main"><div className="content">{children}</div></main>;
}

const preview: Preview = {
  globalTypes: {
    theme: {
      description: "System C theme",
      defaultValue: "dark",
      toolbar: { icon: "mirror", items: ["dark", "light"] },
    },
  },
  decorators: [(Story, context) => <ThemeFrame theme={context.globals.theme}><Story /></ThemeFrame>],
  parameters: {
    a11y: { test: "error" },
    backgrounds: { disable: true },
    controls: { expanded: true },
    layout: "fullscreen",
    viewport: {
      options: {
        desktop: { name: "Desktop", styles: { width: "1280px", height: "900px" } },
        mobile: { name: "Mobile", styles: { width: "390px", height: "844px" } },
      },
    },
  },
};

export default preview;
