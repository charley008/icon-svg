"""Package the file-backed library for import into an existing installation."""

import argparse
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("output", type=Path)
args = parser.parse_args()
args.output.parent.mkdir(parents=True, exist_ok=True)
library = ROOT / "library"
files = [library / "categories.json"]
for directory in sorted(library.iterdir()):
    if directory.is_dir() and not directory.name.startswith("."):
        if directory.name == "licenses":
            files.extend(sorted(p for p in directory.iterdir() if p.suffix.lower() in {".txt", ".md"}))
        else:
            files.append(directory / "meta.json")
            files.extend(sorted(directory.glob("*.svg")))
with zipfile.ZipFile(args.output, "w", zipfile.ZIP_DEFLATED, compresslevel=6) as archive:
    for file in files:
        if file.is_symlink() or not file.is_file():
            raise ValueError(f"Material must be a regular file: {file}")
        archive.write(file, file.relative_to(ROOT).as_posix())
print(f"Packaged {len(files)} material files: {args.output} ({args.output.stat().st_size} bytes)")
