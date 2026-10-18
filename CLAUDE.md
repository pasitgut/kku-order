@AGENTS.md

# DocFlow (ระบบเอกสารคำสั่งแต่งตั้ง วิทยาลัยการคอมพิวเตอร์ มข.)

Go backend ใน `backend/`, Next.js frontend ใน `src/` ข้อความที่ผู้ใช้เห็นและคอมเมนต์ในโค้ดเขียนเป็นภาษาไทย

## Branch และงานของทีม

- `feat/thaidoc-ocr` คือ branch ที่ขึ้น VM จริง (worktree `~/kku/kku-order-thaidoc`) ส่วน `main` เป็นของเพื่อนร่วมทีม (pasitgut) ทั้งสองฝั่ง merge เข้าหากันเป็นระยะ
- การตัดสินใจเรื่องหน้าตาและพฤติกรรมของระบบอยู่ใน `docs/superpowers/specs/` อ่านไฟล์ล่าสุดก่อนแก้ flow การตรวจเอกสาร สถานะ การผูกรายชื่อ หรือรายงาน
- push ขึ้น GitHub และ deploy เฉพาะเมื่อ Kit สั่ง

## เทสต์

- frontend: `npm test` (node --test) · `npx tsc --noEmit -p .` · `npx eslint src`
- backend: เทสต์ที่แตะฐานข้อมูลจะถูกข้ามถ้าไม่ตั้ง `DOCFLOW_TEST_DSN` ต้องรันกับ MySQL จริงทุกครั้งที่แก้ handler หรือ query:
  ```sh
  docker exec docflow-mysql mysql -uroot -proot-docflow -e "CREATE DATABASE IF NOT EXISTS docflow_test; GRANT ALL ON docflow_test.* TO 'docflow'@'%';"
  cd backend && DOCFLOW_TEST_DSN='docflow:docflow@tcp(127.0.0.1:3307)/docflow_test?charset=utf8mb4&parseTime=True&loc=Local' go test -count=1 ./...
  ```
  `docflow-mysql` ในเครื่องมาจาก `docker-compose.yml` (port 3307, เปิดผ่าน OrbStack) ใช้ฐาน `docflow_test` เสมอ ห้ามรันเทสต์กับฐาน `docflow`
- เทสต์ที่สร้างแถว `extracted_fields` หรือ `appointments` ต้องใส่ `BoundingBoxJSON: "null"` เพราะคอลัมน์เป็น JSON และ MySQL ไม่รับสตริงว่าง
- ใช้ `go vet ./...` แทน `go build ./...` เพราะคำสั่งหลังทิ้งไฟล์ binary ชื่อ `backend/backend` ไว้ในโฟลเดอร์

## VM และ deploy

- VM: `project@10.198.200.62` (Ubuntu, x86_64, Docker) ต้องต่อ VPN "KKU" ก่อน และต้องสั่ง `ssh -B ppp0 …` เพราะ macOS ไม่ส่งทางไป `10.198.x` ผ่าน VPN ให้เอง
- deploy: `scripts/deploy-vm.sh` build image `linux/amd64` บน Mac โดยใช้ hash ของ commit เป็นแท็ก แล้วส่งไป VM และรัน compose ใหม่ สคริปต์จะไม่ยอมทำงานถ้ามีไฟล์ที่ track แล้วแก้ค้างไว้ และสคริปต์ใช้ `ssh` ธรรมดา ถ้าเส้นทางไป VM ยังไม่ผ่าน VPN ให้เพิ่ม `BindInterface ppp0` ใน ssh config ของ host นี้
- เว็บจริง: `docflow.pasitgut.com`

## ข้อมูลที่ OCR ให้มาต่างกันในแต่ละเอกสาร

- วันที่: อ่านด้วย `normalizeThaiDate` / `parseDateForField` (`backend/thai_date.go`) เท่านั้น ห้ามใช้ `time.Parse` กับค่าที่มาจาก OCR หรือจากผู้ใช้
- ชื่อ: แยกตำแหน่งวิชาการออกจากชื่อด้วย `splitThaiHonorific` / `nameAndPosition` (`backend/thai_name.go`) ก่อนเก็บหรือนำไปจับคู่
- ชื่อที่ตรงเป๊ะ (EXACT) และชื่อที่เคยผูกไว้ (ALIAS) ผูกกับบุคคลให้เอง ส่วนชื่อที่ใกล้เคียง (FUZZY) เป็นแค่ข้อเสนอ ห้ามผูกเอง (`asSuggestion` ใน `backend/directory_matching.go`)
