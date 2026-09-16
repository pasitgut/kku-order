"""OneOCR cross-platform — auto without --lang, fallback thai
usage:
    python example.py <image>            # auto
    python example.py <image> --lang thai
    python example.py <image> -o out.txt
"""
import sys
from pathlib import Path
from PIL import Image
from oneocr import OneOCR

def is_gibberish(text: str) -> bool:
    if not text.strip():
        return True
    hebrew = sum(1 for c in text if 0x0590 <= ord(c) <= 0x05FF)
    thai = sum(1 for c in text if 0x0E00 <= ord(c) <= 0x0E7F)
    latin = sum(1 for c in text if c.isascii() and c.isalpha())
    total = len([c for c in text if c.strip()])
    if total == 0:
        return True
    if hebrew / max(total,1) > 0.15:
        return True
    if thai+latin == 0 and total > 10:
        return True
    return False

def main():
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass
    if len(sys.argv) < 2:
        print(__doc__.strip())
        sys.exit(1)
    image_path = Path(sys.argv[1])
    out_path = Path(sys.argv[sys.argv.index("-o")+1]) if "-o" in sys.argv else None
    lang = sys.argv[sys.argv.index("--lang")+1] if "--lang" in sys.argv else None

    ocr = OneOCR(default_rotation=0)
    img = Image.open(image_path)
    result = ocr.recognize(img, language=lang)
    if lang is None and is_gibberish(result.full_text):
        result = ocr.recognize(img, language="thai")
        if is_gibberish(result.full_text):
            result = ocr.recognize(img, language="latin")

    if out_path:
        out_path.write_text(result.full_text, encoding="utf-8")
        print(f"saved: {out_path} ({len(result.lines)} lines) angle={result.image_angle}")
    else:
        print(result.full_text)

if __name__ == "__main__":
    main()
