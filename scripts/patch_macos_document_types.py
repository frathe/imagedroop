#!/usr/bin/env python3
"""Adds CFBundleDocumentTypes to a packaged macOS Info.plist.

`fyne package -os darwin` (see Makefile's package-mac target) generates
Info.plist from a fixed template (fyne.io/fyne/v2/cmd/fyne's
templates/data/Info.plist) that has no hook for document-type / file
association metadata - so this runs as a post-processing step instead of
being something FyneApp.toml can express. See todos.md item 1.

LSHandlerRank is "Alternate" rather than "Owner" or "Default": this app is a
viewer, not the authoritative editor for these formats, so it registers as
available in "Open With" without trying to take over as the system default.
"""
import plistlib
import sys

DOCUMENT_TYPES = [
    {
        "CFBundleTypeName": "Image",
        "CFBundleTypeRole": "Viewer",
        "LSHandlerRank": "Alternate",
        # Content-type UTIs are matched first by macOS; extensions are kept
        # too as a fallback for files with no attached type metadata (e.g.
        # coming from a non-Apple filesystem). public.webp only exists from
        # macOS 11 onward - org.webmproject.webp is the older, widely used
        # convention some third-party apps still declare, kept for older
        # WebP files whose type metadata predates public.webp.
        "LSItemContentTypes": [
            "public.jpeg",
            "public.png",
            "com.compuserve.gif",
            "public.webp",
            "org.webmproject.webp",
        ],
        "CFBundleTypeExtensions": [
            "jpg", "jpeg", "jpe", "jfif", "png", "gif", "webp",
        ],
    }
]


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: patch_macos_document_types.py <path/to/Info.plist>", file=sys.stderr)
        return 1

    path = sys.argv[1]

    with open(path, "rb") as f:
        info = plistlib.load(f)

    info["CFBundleDocumentTypes"] = DOCUMENT_TYPES

    with open(path, "wb") as f:
        plistlib.dump(info, f)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
