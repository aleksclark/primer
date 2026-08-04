import importlib.util
import unittest
from pathlib import Path

MODULE_PATH = Path(__file__).with_name("build.py")
SPEC = importlib.util.spec_from_file_location("primer_design_build", MODULE_PATH)
BUILD = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(BUILD)


class BuildTest(unittest.TestCase):
    def setUp(self):
        self.tokens = BUILD.load_tokens()

    def test_theme_contract_and_contrast(self):
        self.assertEqual("dark", self.tokens["meta"]["defaultTheme"])
        BUILD.validate_contrast(self.tokens)

    def test_platform_outputs_include_both_themes(self):
        css = BUILD.css_output(self.tokens)
        kotlin = BUILD.kotlin_output(self.tokens)
        presentation = BUILD.presentation_output(self.tokens)
        self.assertIn('[data-theme="light"]', css)
        self.assertIn("object Dark", kotlin)
        self.assertIn("object Light", kotlin)
        self.assertIn('"defaultTheme": "dark"', presentation)

    def test_contrast_rejects_inaccessible_text(self):
        tokens = BUILD.load_tokens()
        tokens["color"]["dark"]["text"] = tokens["color"]["dark"]["surface"]
        with self.assertRaisesRegex(ValueError, "contrast"):
            BUILD.validate_contrast(tokens)


if __name__ == "__main__":
    unittest.main()
