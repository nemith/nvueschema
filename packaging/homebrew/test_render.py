import hashlib
import tempfile
import unittest
from pathlib import Path

from render import render, would_downgrade


class FormulaTests(unittest.TestCase):
    def test_release_archive_is_pinned(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "source.tar.gz"
            archive.write_bytes(b"release source")
            template = Path(__file__).with_name("nvueschema.rb.in").read_text()
            formula = render("v1.2.3", archive, template)
        self.assertIn('version "1.2.3"', formula)
        self.assertIn("archive/refs/tags/v1.2.3.tar.gz", formula)
        self.assertIn(hashlib.sha256(b"release source").hexdigest(), formula)
        self.assertNotIn("@SHA256@", formula)

    def test_prereleases_cannot_replace_stable_formula(self):
        for tag in ("v1.2.3-rc.1", "main", "v01.2.3", "v1.2.3\n"):
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                render(tag, Path("unused"), "")

    def test_local_url_cannot_inject_ruby(self):
        with self.assertRaises(ValueError):
            render("v1.2.3", Path("unused"), "", 'file:///tmp/#{system("bad")}')

    def test_maintenance_release_does_not_downgrade_tap(self):
        self.assertTrue(would_downgrade('  version "1.10.0"', "v1.9.1"))
        self.assertFalse(would_downgrade('  version "1.9.0"', "v1.10.0"))
        self.assertFalse(would_downgrade('  version "1.10.0"', "v1.10.0"))


if __name__ == "__main__":
    unittest.main()
