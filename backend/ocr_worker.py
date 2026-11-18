import argparse
import csv
import json
import sys
import traceback
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
    try:
        pages = structured_import(Path(args.input_file), source_type)
    except Exception as error:  # noqa: BLE001 - ทุกความผิดพลาดต้องกลายเป็นข้อความสั้นที่ผู้ใช้อ่านได้
        # backend แสดงเฉพาะบรรทัดนี้ให้ผู้ใช้ ส่วน traceback เต็มอยู่ใน log ของ backend
        traceback.print_exc()
        print(f"{ERROR_PREFIX}{user_message(error, source_type)}", file=sys.stderr)
        return 2
    Path(args.output_json).write_text(json.dumps({"pages": pages}, ensure_ascii=False), encoding="utf-8")
    return 0


ERROR_PREFIX = "DOCFLOW_IMPORT_ERROR: "


def user_message(error: Exception, source_type: str) -> str:
    if isinstance(error, UnicodeDecodeError):
        return "ไฟล์ CSV ต้องบันทึกเป็น UTF-8"
    if isinstance(error, (zipfile.BadZipFile, KeyError, ET.ParseError)):
        return f"ไฟล์ {source_type.upper()} เสียหายหรือไม่ใช่รูปแบบที่รองรับ"
    if isinstance(error, csv.Error):
        return "รูปแบบไฟล์ CSV ไม่ถูกต้อง"
    return f"อ่านไฟล์ {source_type.upper()} ไม่ได้"


def structured_import(path: Path, source_type: str):
    if source_type == "csv":
        with path.open("r", encoding="utf-8-sig", newline="") as stream:
            lines = ["\t".join(row) for row in csv.reader(stream)]
    elif source_type == "docx":
        lines = read_docx_lines(path)
    elif source_type == "xlsx":
        lines = read_xlsx_lines(path)
    else:
        raise RuntimeError(f"unsupported structured file type: {source_type}")
    lines = [line for line in lines if line.strip()]
    return [{"pageNo": 1, "imageWidth": 0, "imageHeight": 0, "imageAngle": 0.0, "fullText": "\n".join(lines), "lines": [{"text": line, "confidence": 1.0, "boundingBox": []} for line in lines]}]


def read_docx_lines(path: Path):
    namespace = "{http://schemas.openxmlformats.org/wordprocessingml/2006/main}"
    with zipfile.ZipFile(path) as archive:
        root = ET.fromstring(archive.read("word/document.xml"))

    def paragraph_text(paragraph):
        return "".join(node.text or "" for node in paragraph.findall(f".//{namespace}t")).strip()

    lines = []

    def walk(element):
        for child in element:
            if child.tag == f"{namespace}p":
                lines.append(paragraph_text(child))
            elif child.tag == f"{namespace}tbl":
                # แถวตารางออกมาเป็นบรรทัดเดียวคั่นคอลัมน์ด้วย tab แบบเดียวกับ CSV/XLSX
                for row in child.findall(f"{namespace}tr"):
                    cells = [" ".join(text for text in (paragraph_text(p) for p in cell.findall(f".//{namespace}p")) if text) for cell in row.findall(f"{namespace}tc")]
                    lines.append("\t".join(cells))
            else:
                walk(child)

    body = root.find(f"{namespace}body")
    walk(body if body is not None else root)
    return [line for line in lines if line.strip()]


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
                    # XLSX ไม่เขียนเซลล์ว่างลงไฟล์ ต้องเติมช่องว่างตามคอลัมน์ใน r (เช่น C5) ไม่งั้นคอลัมน์ถัดไปจะเลื่อนมาแทน
                    column = xlsx_column_index(cell.attrib.get("r", ""))
                    if column is not None:
                        while len(values) < column:
                            values.append("")
                    values.append(text or "")
                if values:
                    lines.append("\t".join(values))
        return lines


def xlsx_column_index(reference: str):
    """แปลงตัวอักษรคอลัมน์ในอ้างอิงเซลล์ (เช่น "C5") เป็นลำดับเริ่มที่ 0 ถ้าไม่มีให้คืน None"""
    letters = ""
    for char in reference:
        if "A" <= char.upper() <= "Z":
            letters += char.upper()
        else:
            break
    if not letters:
        return None
    index = 0
    for char in letters:
        index = index * 26 + (ord(char) - ord("A") + 1)
    return index - 1


if __name__ == "__main__":
    raise SystemExit(main())
