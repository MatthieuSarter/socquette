#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.13"
# dependencies = ["pillow>=11"]
# ///
"""Draw the Socquette icon (a striped sock) into internal/assets/icon.png and winres/icon.png.

The drawing uses a 256x256 unit grid, rendered at a high resolution and then downscaled, so edges stay smooth.
"""

from pathlib import Path

from PIL import Image, ImageDraw

UNIT = 256
SCALE = 8
ROOT = Path(__file__).resolve().parent.parent
OUTPUTS = [ROOT / "internal" / "assets" / "icon.png", ROOT / "winres" / "icon.png"]

BACKGROUND = "#2B4C7E"
SOCK = "#F2EDE4"
STRIPE = "#E2574C"
TOE = "#D9D0C1"


def s(*points: float) -> list[float]:
    """Scale unit-grid coordinates to the high-resolution canvas.

    Args:
        *points: Coordinates in the 256-unit grid.

    Returns:
        The coordinates in canvas pixels.
    """
    return [p * SCALE for p in points]


def draw_icon() -> Image.Image:
    """Draw the icon at UNIT * SCALE pixels.

    Returns:
        The RGBA master image.
    """
    img = Image.new("RGBA", (UNIT * SCALE, UNIT * SCALE), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    d.rounded_rectangle(s(8, 8, 248, 248), radius=48 * SCALE, fill=BACKGROUND)
    # Leg: a column; foot: a rounded bar pointing left.
    d.polygon(s(112, 36, 184, 36, 184, 150, 112, 150), fill=SOCK)
    d.rounded_rectangle(s(48, 128, 184, 204), radius=38 * SCALE, fill=SOCK)
    d.pieslice(s(128, 110, 200, 182), start=270, end=90, fill=SOCK)
    # Toe patch.
    d.pieslice(s(48, 128, 110, 204), start=90, end=270, fill=TOE)
    # Cuff stripes.
    for top in (44, 70):
        d.rectangle(s(112, top, 184, top + 14), fill=STRIPE)
    return img


def main() -> None:
    """Render and save the icons."""
    icon = draw_icon().resize((256, 256), Image.Resampling.LANCZOS)
    for path in OUTPUTS:
        path.parent.mkdir(parents=True, exist_ok=True)
        icon.save(path)


if __name__ == "__main__":
    main()
