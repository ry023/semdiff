"""Regression checks for the tagpr skill version hook."""

from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("sync_skill_versions.py")
SKILL_PATHS = (
    "skills/semantic-grouping/SKILL.md",
    "skills/answer-semdiff/SKILL.md",
)


class SyncSkillVersionsTest(unittest.TestCase):
    def test_updates_both_skills_for_next_minor_and_is_idempotent(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "scripts").mkdir()
            shutil.copyfile(SCRIPT, root / "scripts/sync_skill_versions.py")
            (root / "VERSION").write_text("0.5.0\n", encoding="utf-8")
            for relative_path in SKILL_PATHS:
                path = root / relative_path
                path.parent.mkdir(parents=True)
                path.write_text(
                    "This plugin 0.4.x requires CLI >=0.4.0, <0.5.0.\n",
                    encoding="utf-8",
                )

            command = [sys.executable, str(root / "scripts/sync_skill_versions.py")]
            subprocess.run(command, check=True)
            for relative_path in SKILL_PATHS:
                self.assertEqual(
                    (root / relative_path).read_text(encoding="utf-8"),
                    "This plugin 0.5.x requires CLI >=0.5.0, <0.6.0.\n",
                )
            subprocess.run(command, check=True)
            for relative_path in SKILL_PATHS:
                self.assertEqual(
                    (root / relative_path).read_text(encoding="utf-8"),
                    "This plugin 0.5.x requires CLI >=0.5.0, <0.6.0.\n",
                )

    def test_rejects_missing_range(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "scripts").mkdir()
            shutil.copyfile(SCRIPT, root / "scripts/sync_skill_versions.py")
            (root / "VERSION").write_text("0.5.0\n", encoding="utf-8")
            for relative_path in SKILL_PATHS:
                path = root / relative_path
                path.parent.mkdir(parents=True)
                path.write_text("This plugin 0.4.x has no range.\n", encoding="utf-8")

            result = subprocess.run(
                [sys.executable, str(root / "scripts/sync_skill_versions.py")],
                capture_output=True,
                text=True,
            )
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("expected one CLI range", result.stderr)


if __name__ == "__main__":
    unittest.main()
