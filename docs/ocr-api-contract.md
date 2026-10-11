# สัญญา API ของ DocFlow contract (docflow-contract)

เอกสารนี้อธิบายรูปแบบ HTTP และ JSON ที่ DocFlow ใช้คุยกับบริการ OCR ภายนอกที่ใช้ engine `docflow-contract` โค้ดฝั่ง Go อยู่ที่ `backend/extractor_contract.go`

## 1. ภาพรวม

- ส่งไฟล์ด้วย `POST {URL}` แบบ multipart โดยใช้ field ชื่อ `file`
- ถ้ามี key ให้ส่ง header `Authorization: Bearer <key>`
- ค่า API URL ในหน้าตั้งค่าของ DocFlow คือ **URL เต็มของ extract endpoint** (เช่น `https://ocr.example.com/extract`) ระบบไม่ได้ต่อ path เพิ่มให้
- บริการ docflow-contract เปิด `GET /health` ที่ origin ของบริการ (`scheme://host/health`) ซึ่งหมายความว่า health check อยู่ที่ราก origin ไม่ได้อยู่ใต้ path ของ extract endpoint

## 2. โครงสร้าง JSON

| ฟิลด์ | ชนิด | หมายเหตุ |
|---|---|---|
| `engine` | string | ระบุชื่อ engine เช่น `docflow-contract` |
| `header` | object | ข้อมูลหัวคำสั่ง |
| `header.orderNo` | value | เลขที่คำสั่ง |
| `header.subject` | value | เรื่อง |
| `header.issuedBy` | value | ผู้ออกคำสั่ง |
| `header.date` | value | วันที่ออกคำสั่ง |
| `header.effectiveDate` | value | วันที่มีผลบังคับใช้ |
| `header.expiryDate` | value | วันที่สิ้นสุด |
| `header.signer` | value | ผู้ลงนาม |
| `header.references[]` | value[] | เลขที่อ้างอิง |
| `groups[]` | object[] | **ต้องมี key นี้** (ค่าเป็น array ว่างได้) |
| `groups[].department` | value | หน่วยงาน |
| `groups[].duty` | value | หน้าที่ |
| `groups[].members[]` | object[] | รายชื่อบุคลากร |
| `groups[].members[].prefix` | value | คำนำหน้า |
| `groups[].members[].name` | value | ชื่อ-สกุล |
| `groups[].members[].position` | value | ตำแหน่ง |
| `groups[].members[].role` | value | บทบาท เช่น ประธาน |
| `groups[].members[].exOfficio` | boolean | ตำแหน่งโดยตำแหน่ง |
| `pages[]` | object[] | ไม่บังคับ |
| `pages[].page` | number | หมายเลขหน้า เริ่มที่ 1 |
| `pages[].width`, `pages[].height` | number | ขนาดภาพหน้า (พิกเซล) |
| `pages[].lines[]` | value[] | บรรทัดข้อความในหน้า |

## 3. Value object

ทุกค่าข้อความใช้รูปแบบเดียวกัน:

```json
{"text": "1234/2569", "confidence": 0.94, "boxes": [{"page": 1, "x0": 0.4, "y0": 0.1, "x1": 0.6, "y1": 0.12}]}
```

- `text` คือข้อความที่อ่านได้ ถ้าไม่พบให้เป็นสตริงว่าง
- `confidence` เป็นค่า 0 ถึง 1 หรือไม่ส่งก็ได้ ถ้าไม่ส่ง ระบบถือว่าไม่ทราบค่าความมั่นใจ
- `boxes` เป็นพิกัดแบบ normalized ค่า 0 ถึง 1 เริ่มจากมุมซ้ายบน (`x0`, `y0` คือมุมซ้ายบน และ `x1`, `y1` คือมุมขวาล่าง) และระบุ `page`

## 4. กฎการตีความ

- ถ้า `confidence` ของค่าใดไม่ได้ส่งมา ระบบถือว่าไม่ทราบค่า และเอกสารต้องผ่านการตรวจโดยคน
- `exOfficio` เป็น `true` เมื่อ `name` ว่างและ `position` มีค่า
- ถ้าไม่มี key `groups` ใน JSON ระบบจะปฏิเสธคำตอบว่าไม่ใช่ contract

## 5. Health check

- `GET {origin}/health` ตอบ HTTP 200 พร้อม JSON `{"models_loaded": true}` เมื่อโมเดลพร้อมใช้งาน หรือ `{"models_loaded": false}` เมื่อยังไม่พร้อม
- ถ้า JSON ไม่มีฟิลด์ `models_loaded` ถือว่าพร้อมใช้งาน

## 6. ตัวอย่างเต็ม

ดูตัวอย่างคำตอบที่ `backend/testdata/extraction/docflow-contract.json` ไฟล์นี้ใช้ในเทสต์ `backend/extractor_contract_test.go`

## 7. HTTP status และการลองใหม่

| Status | ความหมาย | การจัดการ |
|---|---|---|
| 2xx | สำเร็จ | อ่านคำตอบด้วย `parseContractResponse` |
| 400, 413, 415, 422 | ไฟล์ไม่ถูกต้องถาวร | ไม่ลองใหม่ |
| 401, 403 | key ผิดหรือไม่มีสิทธิ์ | ไม่ลองใหม่ ให้ผู้ดูแลตรวจ key |
| 429, 502, 503, 504 | บริการยุ่งหรือไม่พร้อมชั่วคราว | ลองใหม่ได้ |
| การเชื่อมต่อล้มเหลว | ไม่ได้รับคำตอบ | ลองใหม่ได้ |
| 500 | ข้อผิดพลาดภายในบริการ | ครั้งแรกลองใหม่ ถ้ายังล้มเหลวอีกครั้งถือเป็นถาวร |
