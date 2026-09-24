"""Render the README header: an ukiyo-e style print with Mount Fuji, a red sun,
mist bands, seigaiha waves and a title cartouche.

    python scripts/header.py            # writes docs/assets/header.png

Needs Pillow and numpy. Output is deterministic for a given seed.
"""

from __future__ import annotations

import math
import os
import random
import sys
from pathlib import Path

import numpy as np
from PIL import Image, ImageDraw, ImageFilter, ImageFont

W, H = 1280, 320          # final size
S = 2                     # supersampling factor
w, h = W * S, H * S
SEED = 7

rng = np.random.default_rng(SEED)
random.seed(SEED)

INK = (26, 24, 28)
PAPER = (244, 238, 226)
SUN = (196, 50, 38)
SEAL = (176, 36, 32)
OCHRE = (222, 181, 88)
WAVE = (30, 58, 104)
WAVE_LINE = (228, 233, 236)
MIST = (238, 222, 200)

FONT_DIRS = [
    Path(os.environ.get("WINDIR", r"C:\Windows")) / "Fonts",
    Path("/Library/Fonts"),
    Path("/System/Library/Fonts"),
    Path("/usr/share/fonts"),
]


def font(names: list[str], size: int) -> ImageFont.FreeTypeFont | ImageFont.ImageFont:
    for d in FONT_DIRS:
        if not d.exists():
            continue
        for name in names:
            for p in [d / name, *d.rglob(name)]:
                if p.is_file():
                    return ImageFont.truetype(str(p), size)
    print(f"warning: none of {names} found, using the default font", file=sys.stderr)
    return ImageFont.load_default(size)


def gradient(stops: list[tuple[float, tuple[int, int, int]]], height: int, width: int) -> np.ndarray:
    t = np.linspace(0, 1, height)[:, None]
    pos = np.array([p for p, _ in stops])
    cols = np.array([c for _, c in stops], float)
    out = np.empty((height, width, 3))
    for ch in range(3):
        out[..., ch] = np.interp(t, pos, cols[:, ch])
    return out


def smooth_noise(cells_y: int, cells_x: int) -> np.ndarray:
    """Low-frequency noise in [-1, 1] at full resolution."""
    small = Image.fromarray((rng.uniform(0, 255, (cells_y, cells_x))).astype(np.uint8))
    return np.asarray(small.resize((w, h), Image.BICUBIC), float) / 127.5 - 1


EDGE_NOISE = smooth_noise(h // 5, w // 5)


def rough(mask: Image.Image, amount: float = 70) -> Image.Image:
    """Wobble a mask's edges like a hand-cut woodblock."""
    m = np.asarray(mask.filter(ImageFilter.GaussianBlur(1.5 * S)), float)
    m = m + EDGE_NOISE * amount
    return Image.fromarray(np.clip((m - 128) * 6 + 128, 0, 255).astype(np.uint8))


def paint(img: Image.Image, color, mask: Image.Image, alpha: float = 1.0) -> None:
    if alpha < 1:
        mask = Image.fromarray((np.asarray(mask, float) * alpha).astype(np.uint8))
    img.paste(Image.new("RGB", img.size, color), (0, 0), mask)


def new_mask() -> tuple[Image.Image, ImageDraw.ImageDraw]:
    m = Image.new("L", (w, h), 0)
    return m, ImageDraw.Draw(m)


# --- sky: bokashi gradient, dark indigo at the top fading to a warm horizon
sky = gradient([(0.0, (22, 40, 80)), (0.24, (42, 78, 124)), (0.52, (132, 164, 184)), (0.8, (228, 196, 158))], h, w)
img = Image.fromarray(sky.astype(np.uint8))

# --- sun
sx, sy, sr = 0.625 * w, 0.37 * h, 0.115 * h
m, d = new_mask()
d.ellipse([sx - sr, sy - sr, sx + sr, sy + sr], fill=255)
paint(img, SUN, rough(m, 40))

# --- cranes
for bx, by, bs in [(0.44, 0.17, 1.0), (0.475, 0.23, 0.8), (0.505, 0.15, 0.9), (0.53, 0.26, 0.7), (0.405, 0.27, 0.75)]:
    x, y, s = bx * w, by * h, 9 * S * bs
    m, d = new_mask()
    for side in (-1, 1):
        pts = [(x + side * s * u, y - s * 0.55 * math.sin(math.pi * u) + s * 0.15 * u) for u in np.linspace(0, 1, 12)]
        d.line(pts, fill=255, width=max(2, int(1.6 * S * bs)), joint="curve")
    paint(img, PAPER, m)

# --- Mount Fuji
peak_x, top, base = 0.80 * w, 0.2 * h, 0.82 * h
flat, half = 0.018 * w, 0.2 * w


def slope(t: float) -> float:
    return top + (base - top) * (1 - (1 - t) ** 2.2)


left = [(peak_x - flat - half * t, slope(t)) for t in np.linspace(1, 0, 60)]
right = [(peak_x + flat + half * t, slope(t)) for t in np.linspace(0, 1, 60)]
crater = [(peak_x - flat + 2 * flat * u, top + (3 * S if 0.3 < u < 0.7 else 0) + rng.uniform(-1, 1) * S) for u in np.linspace(0, 1, 7)]
mountain = left + crater + right + [(peak_x + flat + half, h), (peak_x - flat - half, h)]
m_fuji, d = new_mask()
d.polygon(mountain, fill=255)
m_fuji = rough(m_fuji, 45)
body = Image.fromarray(gradient([(0.0, (38, 56, 94)), (0.45, (58, 84, 124)), (0.8, (120, 142, 168)), (1.0, (150, 170, 190))], h, w).astype(np.uint8))
img.paste(body, (0, 0), m_fuji)

# snow cap with finger-like streaks
snow_y = top + 0.3 * (base - top)
x_l = peak_x - flat - half * (1 - (1 - 0.3) ** (1 / 2.2))
x_r = 2 * peak_x - x_l
cuts = np.sort(rng.uniform(x_l, x_r, 10))
shoulders = np.concatenate([[x_l - 6 * S], cuts, [x_r + 6 * S]])
edge = []
for a, b in zip(shoulders[:-1], shoulders[1:]):
    depth = rng.choice([0.03, 0.06, 0.1, 0.16]) * h * rng.uniform(0.7, 1.2)
    tip = a + (b - a) * rng.uniform(0.35, 0.65)
    y0 = snow_y - rng.uniform(0, 0.025) * h
    edge += [(a, y0),
             (a + (tip - a) * 0.75, y0 + depth * 0.45),   # concave sides taper the streak
             (tip, y0 + depth),
             (b - (b - tip) * 0.75, y0 + depth * 0.45)]
edge.append((shoulders[-1], snow_y))
m_snow, d = new_mask()
d.polygon(edge + [(x_r + 6 * S, top - 10 * S), (x_l - 6 * S, top - 10 * S)], fill=255)
m_snow = Image.fromarray((np.asarray(rough(m_snow, 35), float) * np.asarray(m_fuji) / 255).astype(np.uint8))
paint(img, PAPER, m_snow)
# a faint shadow on the right flank of the snow
m, d = new_mask()
d.polygon([(peak_x + flat * 0.3, top), (x_r + 8 * S, snow_y + 0.15 * h), (peak_x + 0.02 * w, snow_y + 0.15 * h)], fill=255)
paint(img, (206, 214, 226), Image.fromarray((np.asarray(m, float) * np.asarray(m_snow) / 255).astype(np.uint8)), 0.6)

# --- mist bands (kasumi)
for x0, x1, yc, bh in [(0.5, 0.74, 0.47, 0.055), (0.7, 1.02, 0.66, 0.065), (0.57, 0.8, 0.735, 0.05), (-0.02, 0.36, 0.72, 0.06)]:
    m, d = new_mask()
    d.rounded_rectangle([x0 * w, (yc - bh / 2) * h, x1 * w, (yc + bh / 2) * h], radius=bh * h / 2, fill=255)
    paint(img, MIST, rough(m, 30), 0.93)

# --- seigaiha waves
R = int(0.078 * h)
wave_top = int(0.83 * h)
row = 0
y = wave_top
while y - R < h:
    offset = R if row % 2 else 0
    for x in range(-2 * R + offset, w + 2 * R, 2 * R):
        jitter = rng.integers(-5, 6)
        fill = tuple(int(c + jitter) for c in WAVE)
        d = ImageDraw.Draw(img)
        for k in range(4):
            r = R * (1 - k * 0.235)
            d.ellipse([x - r, y - r, x + r, y + r], fill=fill, outline=WAVE_LINE, width=int(1.4 * S))
    y += R // 2
    row += 1

# --- title cartouche
f_title = font(["palab.ttf", "Palatino.ttc", "georgiab.ttf", "DejaVuSerif-Bold.ttf"], 88 * S)
f_sub_b = font(["palab.ttf", "Palatino.ttc", "georgiab.ttf", "DejaVuSerif-Bold.ttf"], 30 * S)
f_sub = font(["pala.ttf", "Palatino.ttc", "georgia.ttf", "DejaVuSerif.ttf"], 23 * S)
f_kanji = font(["YuGothB.ttc", "msgothic.ttc", "NotoSansCJK-Bold.ttc", "ヒラギノ角ゴシック W6.ttc"], 19 * S)

cx0, cy0 = 52 * S, 50 * S
probe = ImageDraw.Draw(img)
tw = probe.textbbox((0, 0), "dw", font=f_title)
s1 = probe.textbbox((0, 0), "DocuWare", font=f_sub_b)
s2 = probe.textbbox((0, 0), "from the terminal", font=f_sub)
pad = 26 * S
sub_w = max(s1[2], s2[2])
cw = pad + (tw[2] - tw[0]) + pad + 2 * S + pad + sub_w + pad
ch = 132 * S
m, d = new_mask()
d.rectangle([cx0, cy0, cx0 + cw, cy0 + ch], fill=255)
paint(img, OCHRE, rough(m, 25))
d = ImageDraw.Draw(img)
d.rectangle([cx0 + 5 * S, cy0 + 5 * S, cx0 + cw - 5 * S, cy0 + ch - 5 * S], outline=INK, width=int(2.5 * S))
d.rectangle([cx0 + 11 * S, cy0 + 11 * S, cx0 + cw - 11 * S, cy0 + ch - 11 * S], outline=INK, width=int(1 * S))

tx = cx0 + pad - tw[0]
d.text((tx, cy0 + ch / 2 - (tw[1] + tw[3]) / 2 - 2 * S), "dw", font=f_title, fill=INK)
rule_x = cx0 + pad + (tw[2] - tw[0]) + pad
d.line([(rule_x, cy0 + 30 * S), (rule_x, cy0 + ch - 30 * S)], fill=INK, width=int(1.5 * S))
sx0 = rule_x + pad
d.text((sx0, cy0 + 36 * S), "DocuWare", font=f_sub_b, fill=INK)
d.text((sx0, cy0 + 74 * S), "from the terminal", font=f_sub, fill=INK)

# --- seal stamp (文書, "documents") over the cartouche corner
seal = 50 * S
stamp = Image.new("RGB", (seal, seal), SEAL)
sd = ImageDraw.Draw(stamp)
boxes = [sd.textbbox((0, 0), c, font=f_kanji) for c in "文書"]
gap = 2 * S
cy = (seal - sum(b[3] - b[1] for b in boxes) - gap) / 2
for c, bb in zip("文書", boxes):
    sd.text(((seal - (bb[2] - bb[0])) / 2 - bb[0], cy - bb[1]), c, font=f_kanji, fill=PAPER)
    cy += bb[3] - bb[1] + gap
# worn ink: a few pale speckles, like a real stamp
speckle = rng.uniform(0, 1, (seal, seal)) > 0.94
alpha = Image.fromarray(np.where(speckle, 110, 255).astype(np.uint8))
stamp = stamp.rotate(-5, resample=Image.BICUBIC, expand=True, fillcolor=SEAL)
alpha = alpha.rotate(-5, resample=Image.BICUBIC, expand=True)
sx_, sy_ = int(cx0 + cw - seal * 0.62), int(cy0 + ch - seal * 0.6)
img.paste(stamp, (sx_, sy_), alpha)

# --- paper: wood grain, fibres and grain, then a soft vignette
arr = np.asarray(img, float)
grain_small = rng.normal(0, 1, (h, w // 90))
wood = np.asarray(Image.fromarray(((grain_small + 4) * 32).clip(0, 255).astype(np.uint8)).resize((w, h), Image.BILINEAR), float) / 32 - 4
arr += wood[..., None] * 2.2
arr += rng.normal(0, 5, (h, w, 1))
yy, xx = np.mgrid[0:h, 0:w]
vig = 1 - 0.08 * (((xx - w / 2) / (w / 2)) ** 2 + ((yy - h / 2) / (h / 2)) ** 2)
arr *= vig[..., None]
img = Image.fromarray(arr.clip(0, 255).astype(np.uint8))

fib = Image.new("L", (w, h), 0)
fd = ImageDraw.Draw(fib)
for _ in range(160):
    x, y = rng.uniform(0, w), rng.uniform(0, h)
    a, length = rng.uniform(0, math.pi), rng.uniform(8, 30) * S
    pts = [(x + math.cos(a + 0.4 * math.sin(u * 3)) * length * u, y + math.sin(a + 0.4 * math.sin(u * 3)) * length * u) for u in np.linspace(0, 1, 8)]
    fd.line(pts, fill=int(rng.uniform(8, 20)), width=S)
paint(img, (255, 250, 240), fib)

out = Path(__file__).resolve().parent.parent / "docs" / "assets" / "header.png"
out.parent.mkdir(parents=True, exist_ok=True)
img = img.resize((W, H), Image.LANCZOS)
img = img.quantize(colors=256, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.FLOYDSTEINBERG)
img.save(out, optimize=True)
print(f"wrote {out} ({out.stat().st_size // 1024} KB)")
