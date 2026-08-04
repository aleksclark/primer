from __future__ import annotations

import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent
TOKENS_PATH = ROOT / "tokens.json"
GENERATED = ROOT / "generated"
PREVIEW = ROOT / "preview"


def load_tokens() -> dict:
    tokens = json.loads(TOKENS_PATH.read_text())
    required_themes = {"dark", "light"}
    required_colors = {
        "surface", "surfaceRaised", "rule", "ruleStrong", "textMuted",
        "text", "accent", "accentHover", "onAccent", "attention",
    }
    if tokens.get("meta", {}).get("defaultTheme") not in required_themes:
        raise ValueError("meta.defaultTheme must be dark or light")
    for theme in required_themes:
        missing = required_colors - set(tokens.get("color", {}).get(theme, {}))
        if missing:
            raise ValueError(f"{theme} theme is missing: {', '.join(sorted(missing))}")
    return tokens


def rgb(hex_color: str) -> tuple[int, int, int]:
    if not re.fullmatch(r"#[0-9A-Fa-f]{6}", hex_color):
        raise ValueError(f"Invalid color: {hex_color}")
    return tuple(int(hex_color[index:index + 2], 16) for index in (1, 3, 5))


def luminance(hex_color: str) -> float:
    channels = []
    for channel in rgb(hex_color):
        value = channel / 255
        channels.append(value / 12.92 if value <= 0.04045 else ((value + 0.055) / 1.055) ** 2.4)
    return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2]


def contrast(first: str, second: str) -> float:
    lighter, darker = sorted((luminance(first), luminance(second)), reverse=True)
    return (lighter + 0.05) / (darker + 0.05)


def validate_contrast(tokens: dict) -> None:
    for theme, colors in tokens["color"].items():
        pairs = {
            "text/surface": (colors["text"], colors["surface"]),
            "muted/surface": (colors["textMuted"], colors["surface"]),
            "onAccent/accent": (colors["onAccent"], colors["accent"]),
        }
        for name, values in pairs.items():
            ratio = contrast(*values)
            if ratio < 4.5:
                raise ValueError(f"{theme} {name} contrast is {ratio:.2f}, expected at least 4.5")


def kebab(value: str) -> str:
    return re.sub(r"(?<!^)(?=[A-Z])", "-", value).lower()


def kotlin_name(value: str) -> str:
    parts = re.split(r"[^A-Za-z0-9]+", value)
    result = "".join(part[:1].upper() + part[1:] for part in parts if part)
    return f"Value{result}" if result[:1].isdigit() else result


def css_output(tokens: dict) -> str:
    blocks = []
    for theme in ("dark", "light"):
        selector = ':root, [data-theme="dark"]' if theme == "dark" else '[data-theme="light"]'
        values = [f"  --primer-color-{kebab(name)}: {value};" for name, value in tokens["color"][theme].items()]
        blocks.append(f"{selector} {{\n" + "\n".join(values) + "\n}")
    shared = []
    for name, value in tokens["typography"]["family"].items():
        shared.append(f"  --primer-font-{kebab(name)}: {value};")
    for name, style in tokens["typography"]["style"].items():
        shared.extend([
            f"  --primer-type-{kebab(name)}-size: {style['size'] / 16:g}rem;",
            f"  --primer-type-{kebab(name)}-weight: {style['weight']};",
            f"  --primer-type-{kebab(name)}-line-height: {style['lineHeight']};",
            f"  --primer-type-{kebab(name)}-tracking: {style['letterSpacing']};",
        ])
    for group, unit in (("space", "px"), ("radius", "px"), ("rule", "px"), ("focus", "px"), ("motion", "ms")):
        for name, value in tokens[group].items():
            shared.append(f"  --primer-{group}-{kebab(name)}: {value}{unit};")
    return "\n\n".join(blocks) + "\n\n:root {\n" + "\n".join(shared) + "\n}\n"


def kotlin_output(tokens: dict) -> str:
    lines = ["package com.aleksclark.primer.designsystem", "", "import androidx.compose.ui.graphics.Color", "", "object PrimerTokens {"]
    for theme in ("dark", "light"):
        lines.append(f"    object {theme.capitalize()} {{")
        for name, value in tokens["color"][theme].items():
            lines.append(f"        val {name} = Color(0xFF{value[1:].upper()})")
        lines.extend(["    }", ""])
    for group in ("space", "radius", "rule", "focus", "motion"):
        lines.append(f"    object {kotlin_name(group)} {{")
        for name, value in tokens[group].items():
            lines.append(f"        const val {kotlin_name(name)} = {value}")
        lines.extend(["    }", ""])
    lines[-1] = "}"
    return "\n".join(lines) + "\n"


def presentation_output(tokens: dict) -> str:
    return json.dumps({
        "name": tokens["meta"]["name"],
        "system": tokens["meta"]["system"],
        "version": tokens["meta"]["version"],
        "defaultTheme": tokens["meta"]["defaultTheme"],
        "themes": tokens["color"],
        "typography": tokens["typography"],
        "spacingPx": tokens["space"],
        "radiusPx": tokens["radius"],
        "rulesPx": tokens["rule"],
    }, indent=2) + "\n"


def reference_output(tokens: dict) -> str:
    rows = ["# Primer System C token reference", "", f"Version {tokens['meta']['version']}. Dark mode is the default.", ""]
    for theme in ("dark", "light"):
        rows.extend([f"## {theme.capitalize()} colors", "", "| Role | Value |", "|---|---|"])
        rows.extend(f"| `{name}` | `{value}` |" for name, value in tokens["color"][theme].items())
        rows.append("")
    rows.extend(["## Typography", "", "| Style | Family | Size | Weight | Line height | Tracking |", "|---|---|---:|---:|---:|---:|"])
    for name, style in tokens["typography"]["style"].items():
        rows.append(f"| `{name}` | `{style['family']}` | {style['size']} px | {style['weight']} | {style['lineHeight']} | `{style['letterSpacing']}` |")
    return "\n".join(rows) + "\n"


def preview_output() -> str:
    return """<!doctype html>
<html lang="en" data-theme="dark">
<head>
  <meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Primer System C</title><link rel="stylesheet" href="../generated/primer.css">
  <link rel="preconnect" href="https://fonts.googleapis.com"><link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Instrument+Sans:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500&display=swap" rel="stylesheet">
  <style>
    *{box-sizing:border-box} body{margin:0;background:var(--primer-color-surface);color:var(--primer-color-text);font-family:var(--primer-font-content)} button{font:inherit}
    .shell{max-width:1200px;margin:auto;padding:48px}.bar{display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--primer-color-rule-strong);padding-bottom:24px}.logo{width:205px;height:60px;object-fit:contain;object-position:left}
    .label{font-family:var(--primer-font-system);font-size:var(--primer-type-label-size);letter-spacing:var(--primer-type-label-tracking);text-transform:uppercase;color:var(--primer-color-text-muted)} h1{max-width:800px;margin:64px 0 16px;font-size:var(--primer-type-display-size);font-weight:500;line-height:1.05;letter-spacing:-.024em}.lede{max-width:660px;color:var(--primer-color-text-muted);font-size:15px;line-height:1.55}
    .actions{display:flex;gap:12px;margin:24px 0 64px}.button{padding:12px 20px;border:1px solid var(--primer-color-rule);border-radius:0;background:transparent;color:var(--primer-color-text);font-family:var(--primer-font-system);font-size:12px;letter-spacing:.06em;text-transform:uppercase}.button.primary{background:var(--primer-color-accent);border-color:var(--primer-color-accent);color:var(--primer-color-on-accent);font-weight:500}.button:hover{border-color:var(--primer-color-text-muted)}.button.primary:hover{background:var(--primer-color-accent-hover)}
    .grid{display:grid;grid-template-columns:repeat(3,1fr);gap:32px}.card{min-height:190px;padding:22px;border:1px solid var(--primer-color-rule);background:var(--primer-color-surface-raised)}.card h2{font-size:18px;margin:14px 0 8px}.card p{color:var(--primer-color-text-muted);font-size:14px;line-height:1.55}.progress{height:3px;margin-top:24px;background:var(--primer-color-rule)}.progress span{display:block;width:72%;height:100%;background:var(--primer-color-accent)}
    .attention{border-left:2px solid var(--primer-color-attention)}.toggle{cursor:pointer;background:none;color:var(--primer-color-text-muted);border:0;border-bottom:1px solid var(--primer-color-text-muted);padding:8px 0;font-family:var(--primer-font-system);font-size:11px;letter-spacing:.08em;text-transform:uppercase}:focus-visible{outline:1px solid var(--primer-color-accent);outline-offset:2px}
    @media(max-width:720px){.shell{padding:24px}.grid{grid-template-columns:1fr}.bar{align-items:flex-start}.logo{width:170px}h1{margin-top:48px}}
  </style>
</head>
<body><main class="shell"><header class="bar"><img class="logo" src="../assets/logo/logo.svg" alt="Primer LMS"><button class="toggle" type="button">Light theme</button></header>
  <section><h1>Mastery, not coverage.</h1><p class="lede">Your tutor will hold this unit until you can explain it back without prompting. Nothing new opens until then.</p><div class="actions"><button class="button primary">Continue</button><button class="button">Review progress</button></div></section>
  <section class="grid"><article class="card"><div class="label">Mastered this week</div><h2>3 standards</h2><p>Measured against demonstrated understanding, not elapsed time.</p><div class="progress"><span></span></div></article><article class="card"><div class="label">Unit 04 / 09</div><h2>Fractions as division</h2><p>Six problems left. Held until the student can explain the step back.</p></article><article class="card attention"><div class="label" style="color:var(--primer-color-attention)">Needs review</div><h2>Word problems</h2><p>Three misses require a parent-facing record and another guided attempt.</p></article></section>
</main><script>const root=document.documentElement,button=document.querySelector('.toggle'),logo=document.querySelector('.logo');button.addEventListener('click',()=>{const light=root.dataset.theme==='light';root.dataset.theme=light?'dark':'light';button.textContent=light?'Light theme':'Dark theme';logo.src=light?'../assets/logo/logo.svg':'../assets/logo/logo-light.svg'})</script></body></html>
"""


def main() -> None:
    tokens = load_tokens()
    validate_contrast(tokens)
    GENERATED.mkdir(exist_ok=True)
    PREVIEW.mkdir(exist_ok=True)
    (GENERATED / "primer.css").write_text(css_output(tokens))
    (GENERATED / "PrimerTokens.kt").write_text(kotlin_output(tokens))
    (GENERATED / "presentation-theme.json").write_text(presentation_output(tokens))
    (GENERATED / "token-reference.md").write_text(reference_output(tokens))
    (PREVIEW / "index.html").write_text(preview_output())
    print("Primer design system generated")


if __name__ == "__main__":
    main()
