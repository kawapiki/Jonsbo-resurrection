"""Rasterize IBM Plex Sans SemiBold with its actual OpenType slashed-zero glyph.
Requires Pillow and fonttools. Input: official IBMPlexSans-SemiBold.ttf.
Only the resulting alpha atlas is distributed; the input font is not modified.
"""
import io
import json
import sys
from pathlib import Path
from fontTools.ttLib import TTFont
from PIL import Image, ImageDraw, ImageFont

out = Path(__file__).resolve().parents[1] / 'modules/configurator/fonts'
tt = TTFont(sys.argv[1])
features = tt['GSUB'].table
record = next(r for r in features.FeatureList.FeatureRecord if r.FeatureTag == 'zero')
mapping = {}
for lookup in record.Feature.LookupListIndex:
    for sub in features.LookupList.Lookup[lookup].SubTable:
        mapping.update(sub.mapping)
for table in tt['cmap'].tables:
    if table.isUnicode() and 48 in table.cmap:
        table.cmap[48] = mapping.get(table.cmap[48], table.cmap[48])
buffer = io.BytesIO()
tt.save(buffer)
buffer.seek(0)
font = ImageFont.truetype(buffer, 512)
top, bottom = font.getbbox('0123456789')[1::2]
scale = 96 / (bottom - top)
chars = '0123456789.-°CPU'
atlas = Image.new('L', (120 * len(chars), 112))
glyphs = {}
assert len({font.getlength(c) for c in '0123456789'}) == 1, 'Need tabular figures'
for index, char in enumerate(chars):
    advance = round(font.getlength(char) * scale)
    tile = Image.new('L', (600, 600))
    ImageDraw.Draw(tile).text((0, -top), char, font=font, fill=255)
    tile = tile.resize((round(600 * scale), round(600 * scale)), Image.Resampling.LANCZOS)
    atlas.paste(tile.crop((0, 0, advance, 112)), (index * 120, 0))
    glyphs[char] = {'x': index * 120, 'w': advance}
atlas.save(out / 'plex-dial.png')
(out / 'plex-dial.json').write_text(json.dumps(glyphs), encoding='utf-8')
print('IBM Plex Sans SemiBold: slashed zero and tabular numerals rasterized.')
