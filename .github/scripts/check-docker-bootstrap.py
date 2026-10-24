"""Check first-run bind directory seeding without using deployment data."""
import os
import pathlib
import subprocess
import sys
import tempfile

image = sys.argv[1]

def start(library, data, uid=None):
    command = ["docker", "run", "--rm", "--read-only", "--cap-drop=ALL"]
    for capability in ("CHOWN", "SETUID", "SETGID"):
        command += ["--cap-add", capability]
    command += ["--mount", f"type=bind,src={library},dst=/app/library",
                "--mount", f"type=bind,src={data},dst=/app/data"]
    if uid:
        command += ["-e", f"PUID={uid}", "-e", f"PGID={uid}"]
    subprocess.run(command + [image, "-version"], check=True)

with tempfile.TemporaryDirectory(prefix="icon-svg-bootstrap-") as temporary:
    root = pathlib.Path(temporary)
    library, data = root / "library", root / "data"
    library.mkdir()
    data.mkdir()
    start(library, data)
    files = list(library.glob("*/*.svg"))
    assert files, "empty bind directory was not initialized"
    assert (library / ".icon-svg-initialized").is_file()
    assert not (library / ".icon-svg-initializing").exists()
    edited = files[0]
    edited.write_bytes(edited.read_bytes() + b"\n<!-- user edit -->\n")
    content = edited.read_bytes()
    removed = files[1]
    removed.unlink()
    start(library, data)
    assert edited.read_bytes() == content, "restart overwrote user artwork"
    assert not removed.exists(), "restart restored a user deletion"

    existing = root / "existing"
    existing.mkdir()
    (existing / "keep.txt").write_text("user library")
    start(existing, data)
    assert sorted(p.name for p in existing.iterdir()) == ["keep.txt"]

    interrupted = root / "interrupted"
    interrupted.mkdir()
    (interrupted / ".icon-svg-initializing").touch()
    first = interrupted / files[0].relative_to(library)
    first.parent.mkdir()
    first.write_bytes(content)
    os.chown(first.parent, 12345, 12345)
    os.chown(interrupted / ".icon-svg-initializing", 12345, 12345)
    start(interrupted, data, uid=12345)
    assert first.read_bytes() == content, "resume overwrote an existing file"
    assert (interrupted / ".icon-svg-initialized").is_file()
    assert len(list(interrupted.glob("*/*.svg"))) == len(files)
    assert not (interrupted / ".icon-svg-initializing").exists()
    print("Bootstrap checks passed: initial fill, restart preservation, nonempty skip, interrupted retry, custom UID.")
