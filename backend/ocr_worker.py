import argparse
import csv
import json
import zipfile
import xml.etree.ElementTree as ET
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser(description="DocFlow structured import worker (DOCX, XLSX, CSV)")
    parser.add_argument("--input-file", required=True)
    parser.add_argument("--output-json", required=True)
    parser.add_argument("--source-type", required=True)
    args = parser.parse_args()

    source_type = args.source_type.lower()
    if source_type == "pdf":
        raise RuntimeError("PDF files are extracted by the external OCR API, not this worker")
    pages = structured_import(Path(args.input_file), source_type)
    Path(args.output_json).write_text(json.dumps({"pages": pages}, ensure_ascii=False), encoding="utf-8")
    return 0


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
