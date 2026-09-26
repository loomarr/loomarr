#!/usr/bin/env python3
"""Channel watermark ("bug") renderer for the #1512 phase 1d design samples. Throwaway spike code.

Every bug is a WHITE silhouette: RGB is white everywhere and only alpha carries the shape, so the
4:2:0 overlay cannot fringe (white has neutral chroma). Opacity is baked into alpha here, once, so
the GPU overlay is a plain alpha blend.

Sources:
  logo:<png>              a TMDB network logo; the silhouette is the logo's own alpha, trimmed.
  plate:<CALL>            callsign knocked out of a rounded white plate (Geist Bold).
  monogram:<L>            one or two letters inside a ring (Geist Bold).
  stacked:<TOP>/<BOTTOM>  two words stacked and justified to one width (Barlow Condensed ExtraBold).
  tag:<NUM>/<CALL>        channel number knocked out of a square, callsign beside it (Barlow Condensed).

Size rule (equal visual weight): every bug covers the same area, 2 * H^2, where H = frame height *
size (default 6%). A wide mark gets shorter and a square one reaches H, capped at H tall. Rendering is
deterministic: shapes are drawn at 8x and reduced with Lanczos; no randomness, no system fonts.

usage: render_bugs.py SOURCE OUT.png [--frame-h 1080] [--size 0.06] [--opacity 0.65] [--shadow]
       --fonts DIR   (Geist-Bold.ttf and BarlowCondensed-ExtraBold.ttf)
"""
import argparse
import math
import os

from PIL import Image, ImageDraw, ImageFilter, ImageFont

SS = 8  # supersample factor


def trim(alpha):
    box = alpha.getbbox()
    return alpha.crop(box) if box else alpha


def text_mask(text, font_path, px):
    """Alpha mask of text at cap height ~px (supersampled), tightly trimmed."""
    font = ImageFont.truetype(font_path, int(px * 1.45))
    l, t, r, b = font.getbbox(text)
    m = Image.new("L", (r - l + 4, b - t + 4), 0)
    ImageDraw.Draw(m).text((2 - l, 2 - t), text, font=font, fill=255)
    return trim(m)


def fit_width(mask, w):
    return mask.resize((w, max(1, round(mask.height * w / mask.width))), Image.LANCZOS)


def logo(path):
    im = Image.open(path).convert("RGBA")
    return trim(im.getchannel("A"))


def plate(call, fonts):
    h = 100 * SS
    txt = text_mask(call, fonts["geist"], h * 0.52)
    pad_x = int(h * 0.30)
    w = txt.width + 2 * pad_x
    m = Image.new("L", (w, h), 0)
    ImageDraw.Draw(m).rounded_rectangle((0, 0, w - 1, h - 1), radius=int(h * 0.22), fill=255)
    knock = Image.new("L", m.size, 0)
    knock.paste(txt, (pad_x, (h - txt.height) // 2))
    return Image.composite(Image.new("L", m.size, 0), m, knock)


def monogram(letters, fonts):
    d = 100 * SS
    ring = int(d * 0.075)
    m = Image.new("L", (d, d), 0)
    ImageDraw.Draw(m).ellipse((0, 0, d - 1, d - 1), outline=255, width=ring)
    txt = text_mask(letters, fonts["geist"], d * 0.46)
    if txt.width > d * 0.62:
        txt = fit_width(txt, int(d * 0.62))
    m.paste(255, ((d - txt.width) // 2, (d - txt.height) // 2), txt)
    return m


def stacked(words, fonts):
    top, bottom = words.split("/", 1)
    w = 300 * SS
    a = fit_width(text_mask(top, fonts["barlow"], 100 * SS), w)
    b = fit_width(text_mask(bottom, fonts["barlow"], 100 * SS), w)
    gap = int(max(a.height, b.height) * 0.14)
    m = Image.new("L", (w, a.height + gap + b.height), 0)
    m.paste(a, (0, 0))
    m.paste(b, (0, a.height + gap))
    return m


def tag(spec, fonts):
    num, call = spec.split("/", 1)
    h = 100 * SS
    box = Image.new("L", (h, h), 0)
    ImageDraw.Draw(box).rounded_rectangle((0, 0, h - 1, h - 1), radius=int(h * 0.14), fill=255)
    n = text_mask(num, fonts["barlow"], h * 0.62)
    if n.width > h * 0.78:
        n = fit_width(n, int(h * 0.78))
    knock = Image.new("L", box.size, 0)
    knock.paste(n, ((h - n.width) // 2, (h - n.height) // 2))
    box = Image.composite(Image.new("L", box.size, 0), box, knock)
    c = text_mask(call, fonts["barlow"], h * 0.80)
    gap = int(h * 0.16)
    m = Image.new("L", (h + gap + c.width, h), 0)
    m.paste(box, (0, 0))
    m.paste(c, (h + gap, (h - c.height) // 2))
    return m


def size_to(mask, frame_h, size):
    H = frame_h * size
    ar = mask.width / mask.height
    h = min(H, H * math.sqrt(2 / ar))
    w = h * ar
    return mask.resize((max(1, round(w)), max(1, round(h))), Image.LANCZOS)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("source")
    ap.add_argument("out")
    ap.add_argument("--frame-h", type=int, default=1080)
    ap.add_argument("--size", type=float, default=0.06)
    ap.add_argument("--opacity", type=float, default=0.65)
    ap.add_argument("--shadow", action="store_true")
    ap.add_argument("--fonts", required=True)
    a = ap.parse_args()
    fonts = {"geist": os.path.join(a.fonts, "Geist-Bold.ttf"),
             "barlow": os.path.join(a.fonts, "BarlowCondensed-ExtraBold.ttf")}
    kind, arg = a.source.split(":", 1)
    mask = {"logo": lambda: logo(arg), "plate": lambda: plate(arg, fonts), "monogram": lambda: monogram(arg, fonts),
            "stacked": lambda: stacked(arg, fonts), "tag": lambda: tag(arg, fonts)}[kind]()
    mask = size_to(mask, a.frame_h, a.size)
    alpha = mask.point(lambda v: round(v * a.opacity))
    white = Image.new("RGBA", mask.size, (255, 255, 255, 0))
    white.putalpha(alpha)
    if not a.shadow:
        white.save(a.out)
        return
    # Soft shadow proposal: black, 35% of the bug's opacity, offset ~1/40 H, blurred ~1/30 H.
    pad = max(4, round(mask.height * 0.12))
    off = max(1, round(mask.height / 40))
    canvas = Image.new("RGBA", (mask.width + 2 * pad, mask.height + 2 * pad), (0, 0, 0, 0))
    sh = Image.new("L", canvas.size, 0)
    sh.paste(mask, (pad + off, pad + off))
    sh = sh.filter(ImageFilter.GaussianBlur(max(1, mask.height / 30))).point(lambda v: round(v * a.opacity * 0.35))
    canvas.putalpha(sh)
    canvas.alpha_composite(white, (pad, pad))
    canvas.save(a.out)


if __name__ == "__main__":
    main()
