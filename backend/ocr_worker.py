import argparse
import csv
import json
import sys
import zipfile
import xml.etree.ElementTree as ET
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser(description="DocFlow local OneOCR worker")
    parser.add_argument("--input-dir", default="")
    parser.add_argument("--input-file")
    parser.add_argument("--output-json", required=True)
    parser.add_argument("--models-dir", default="")
    parser.add_argument("--source-type", default="pdf")
    parser.add_argument("--language", default="auto")
    parser.add_argument("--max-side", type=int, default=1536)
    parser.add_argument("--preprocess", choices=["standard", "none"], default="none")
    args = parser.parse_args()

    if args.source_type.lower() != "pdf":
        if not args.input_file:
            raise RuntimeError("--input-file is required for structured imports")
        pages = structured_import(Path(args.input_file), args.source_type.lower())
        Path(args.output_json).write_text(json.dumps({"pages": pages}, ensure_ascii=False), encoding="utf-8")
        return 0

    package_dir = Path(__file__).resolve().parent / "one-ocr"
    sys.path.insert(0, str(package_dir))
    from oneocr import OneOCR
    from PIL import Image, ImageEnhance, ImageOps

    language = None if args.language.strip().lower() in {"", "auto"} else args.language.strip()
    # Match sample-code: keep the PDF orientation fixed, avoid an extra
    # deskew pass that can move OCR boxes away from the displayed page, and
    # use the detector threshold used by the working mockup.
    ocr = OneOCR(config_dir=Path(args.models_dir), default_rotation=0, max_side=args.max_side, score_threshold=0.5)
    pages = []
    image_paths = sorted(Path(args.input_dir).glob("page-*.png"), key=lambda path: int(path.stem.rsplit("-", 1)[1]))
    for image_path in image_paths:
        page_no = int(image_path.stem.rsplit("-", 1)[1])
        with Image.open(image_path) as image:
            image = prepare_image(image, args.preprocess, ImageEnhance, ImageOps)
            result = ocr.recognize(image, language=language, max_side=args.max_side, score_threshold=0.5)
            lines = []
            for line in result.lines:
                confidence = sum(word.confidence for word in line.words) / len(line.words) if line.words else 0.0
                words = [{
                    "text": word.text,
                    "confidence": word.confidence,
                    "boundingBox": list(word.bbox.as_rect()) if word.bbox else [],
                } for word in line.words]
                lines.append({
                    "text": line.text,
                    "confidence": confidence,
                    "boundingBox": list(line.bbox.as_rect()) if line.bbox else [],
                    "words": words,
                })
            pages.append({
                "pageNo": page_no,
                "imageWidth": image.width,
                "imageHeight": image.height,
                "imageAngle": result.image_angle,
                "fullText": result.full_text,
                "lines": lines,
            })

    if not pages:
        raise RuntimeError("no rendered PDF pages were found")
    output = {"pages": pages}
    Path(args.output_json).write_text(json.dumps(output, ensure_ascii=False), encoding="utf-8")
    return 0


def prepare_image(image, mode, image_enhance, image_ops):
    image = image_ops.exif_transpose(image).convert("RGB")
    if mode == "none":
        return image
    image = deskew_image(image)
    return image_enhance.Brightness(image).enhance(1.05)


def deskew_image(image):
    """Estimate a small text skew angle and rotate the source page once.

    The search runs on a reduced grayscale copy, then applies only the chosen
    rotation to the original RGB page. This intentionally avoids denoising or
    other image changes outside the TOR's deskew/brightness scope.
    """
    import numpy as np
    from PIL import Image as PILImage

    max_dimension = max(image.size)
    scale = min(1.0, 1200.0 / max_dimension)
    preview = image
    if scale < 1.0:
        preview = image.resize((max(1, int(image.width * scale)), max(1, int(image.height * scale))))
    grayscale = np.asarray(preview.convert("L"))
    dark_pixels = grayscale < 200
    if int(dark_pixels.sum()) < 100:
        return image

    best_angle = 0.0
    best_score = -1.0
    for angle in np.arange(-15.0, 15.01, 1.0):
        rotated = preview.rotate(float(angle), resample=PILImage.Resampling.BILINEAR, expand=False, fillcolor=255)
        profile = (np.asarray(rotated.convert("L")) < 200).sum(axis=1)
        score = float(profile.var())
        if score > best_score:
            best_score = score
            best_angle = float(angle)
    if abs(best_angle) < 0.5:
        return image
    return image.rotate(best_angle, resample=PILImage.Resampling.BICUBIC, expand=False, fillcolor="white")


def structured_import(path: Path, source_type: str):
    if source_type == "csv":
        with path.open("r", encoding="utf-8-sig", newline="") as stream:
            lines = ["\t".join(row) for row in csv.reader(stream)]
    elif source_type == "docx":
        namespace = "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}"
        with zipfile.ZipFile(path) as archive:
            root = ET.fromstring(archive.read("word/document.xml"))
        lines = ["".join(node.text or "" for node in paragraph.findall(f".//{namespace}t")).strip() for paragraph in root.findall(f".//{namespace}p")]
        lines = [line for line in lines if line]
    elif source_type == "xlsx":
        lines = read_xlsx_lines(path)
    else:
        raise RuntimeError(f"unsupported structured file type: {source_type}")
    lines = [line for line in lines if line.strip()]
    return [{"pageNo": 1, "imageWidth": 0, "imageHeight": 0, "imageAngle": 0.0, "fullText": "\n".join(lines), "lines": [{"text": line, "confidence": 1.0, "boundingBox": []} for line in lines]}]


def read_xlsx_lines(path: Path):
    namespace = "{http://schemas.openxmlformats.org/spreadsheetml/2006/main}"
    with zipfile.ZipFile(path) as archive:
        shared = []
        if "xl/sharedStrings.xml" in archive.namelist():
            shared_root = ET.fromstring(archive.read("xl/sharedStrings.xml"))
            shared = ["".join(node.text or "" for node in item.findall(f".//{namespace}t")) for item in shared_root.findall(f".//{namespace}si")]
        lines = []
        for name in sorted(item for item in archive.namelist() if item.startswith("xl/worksheets/sheet") and item.endswith(".xml")):
            root = ET.fromstring(archive.read(name))
            for row in root.findall(f".//{namespace}row"):
                values = []
                for cell in row.findall(f"{namespace}c"):
                    value = cell.find(f"{namespace}v")
                    inline = cell.find(f".//{namespace}t")
                    text = inline.text if inline is not None else (value.text if value is not None else "")
                    if cell.attrib.get("t") == "s" and text.isdigit() and int(text) < len(shared):
                        text = shared[int(text)]
                    values.append(text or "")
                if values:
                    lines.append("\t".join(values))
        return lines


if __name__ == "__main__":
    raise SystemExit(main())
