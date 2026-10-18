"""พิมพ์รายงาน DocFlow เป็น PDF: อ่าน HTML จาก stdin แล้วเขียน PDF ออก stdout

ใช้ WeasyPrint (Pango + HarfBuzz) เพราะสระบน สระล่าง และวรรณยุกต์ไทยต้องจัดตำแหน่งตามฟอนต์
ไลบรารี PDF ทั่วไปวางทับกันจนอ่านไม่ออก
"""
import sys

from weasyprint import HTML


def main() -> int:
    page = sys.stdin.buffer.read().decode("utf-8")
    sys.stdout.buffer.write(HTML(string=page).write_pdf())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
