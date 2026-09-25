#!/usr/bin/env python3
"""Check relative Markdown links and their heading anchors.

Usage: python3 hack/check-links.py <file-or-directory>...

Every inline link or image, [text](target) or ![alt](target), and every
reference definition, [label]: target, whose target is relative must name a
file or directory that exists. When it has a #fragment, the target Markdown
file (or the linking file, for a bare #fragment) must have a heading whose
GitHub-style anchor matches. External links (a URL scheme or //host) are not
fetched. Links inside code fences and inline code are ignored.

Prints each broken link as path:line: target: reason, then a count, and exits
1 when any link is broken.
"""

import os
import re
import sys
import unicodedata

FENCE = re.compile(r"^\s*(```|~~~)")
INLINE_CODE = re.compile(r"`+[^`]*`+")
# [text](target "title") and ![alt](target); the text may hold one level of
# brackets, as in [`x[0]`](y).
INLINE_LINK = re.compile(
    r"!?\[(?:[^\[\]]|\[[^\[\]]*\])*\]\(\s*<?([^()\s>]+)>?(?:\s+[\"'][^\"']*[\"'])?\s*\)"
)
REFERENCE = re.compile(r"^\s{0,3}\[[^\]]+\]:\s*<?(\S+?)>?(?:\s+.*)?$")
HEADING = re.compile(r"^\s{0,3}(#{1,6})\s+(.*?)\s*#*\s*$")
EXPLICIT_ID = re.compile(r"""<a\s+(?:id|name)=["']([^"']+)["']|\{#([^}\s]+)\}""")
SCHEME = re.compile(r"^[a-zA-Z][a-zA-Z0-9+.-]*:")


def markdown_files(args):
    for arg in args:
        if os.path.isdir(arg):
            for root, dirs, files in os.walk(arg):
                dirs[:] = sorted(d for d in dirs if not d.startswith("."))
                for name in sorted(files):
                    if name.endswith(".md"):
                        yield os.path.join(root, name)
        else:
            yield arg


def lines_outside_code(path):
    """Yield (number, text) for lines outside code fences, inline code removed."""
    fenced = False
    with open(path, encoding="utf-8") as handle:
        for number, text in enumerate(handle, 1):
            if FENCE.match(text):
                fenced = not fenced
                continue
            if not fenced:
                yield number, INLINE_CODE.sub("", text.rstrip("\n"))


def slug(heading):
    """Return the anchor GitHub gives a heading."""
    text = re.sub(r"!?\[([^\]]*)\]\([^)]*\)", r"\1", heading)  # links -> text
    text = re.sub(r"<[^>]+>", "", text)  # inline HTML
    text = text.strip().lower()
    kept = []
    for char in text:
        category = unicodedata.category(char)
        if char in "-_" or category[0] in "LN" or category == "Mn":
            kept.append(char)
        elif char == " ":
            kept.append("-")
    return "".join(kept)


_anchors = {}


def anchors(path):
    """Return the set of anchors a Markdown file defines."""
    if path not in _anchors:
        found, seen = set(), {}
        fenced = False
        with open(path, encoding="utf-8") as handle:
            for text in handle:
                if FENCE.match(text):
                    fenced = not fenced
                    continue
                if fenced:
                    continue
                for match in EXPLICIT_ID.finditer(text):
                    found.add(match.group(1) or match.group(2))
                heading = HEADING.match(text)
                if heading:
                    base = slug(heading.group(2))
                    count = seen.get(base, 0)
                    seen[base] = count + 1
                    found.add(base if count == 0 else f"{base}-{count}")
        _anchors[path] = found
    return _anchors[path]


def check(path, target):
    """Return why a relative link target is broken, or None."""
    location, _, fragment = target.partition("#")
    location = location.split("?", 1)[0]
    if location:
        resolved = os.path.normpath(os.path.join(os.path.dirname(path), location))
        if not os.path.exists(resolved):
            return "no such file"
    else:
        resolved = path
    if (
        fragment
        and resolved.endswith(".md")
        and os.path.isfile(resolved)
        and fragment.lower() not in anchors(resolved)
    ):
        return f"no heading #{fragment} in {resolved}"
    return None


def main(args):
    if not args:
        print(__doc__.strip().splitlines()[2], file=sys.stderr)
        return 2
    broken = checked = 0
    for path in markdown_files(args):
        for number, text in lines_outside_code(path):
            targets = [m.group(1) for m in INLINE_LINK.finditer(text)]
            reference = REFERENCE.match(text)
            if reference:
                targets.append(reference.group(1))
            for target in targets:
                if SCHEME.match(target) or target.startswith("//"):
                    continue
                checked += 1
                reason = check(path, target)
                if reason:
                    broken += 1
                    print(f"{path}:{number}: {target}: {reason}")
    print(f"{checked} relative links checked, {broken} broken")
    return 1 if broken else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
