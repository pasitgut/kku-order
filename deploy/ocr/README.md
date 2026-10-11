# thaidoc OCR บน VM ทดสอบ

โฟลเดอร์นี้รัน thaidoc-extract (OCR) ไว้หลัง reverse proxy (Caddy) ตัวเดียว
DocFlow จะเรียก thaidoc ผ่าน proxy เท่านั้น ด้วย URL + คีย์
ตอนนี้ทั้งสองอยู่ใน VM เดียว ภายหลังย้าย thaidoc ไปเครื่อง GPU ได้โดยเปลี่ยนแค่ URL

- VM: `project@10.198.200.62` (Ubuntu 24.04, ไม่มี GPU, ใช้ Docker ได้โดยไม่ต้องใช้ sudo)
- โฟลเดอร์บน VM: `~/docflow/kku-order/deploy/ocr/` และซอร์ส `~/docflow/thaidoc-src/`
- พอร์ตที่ใช้: `127.0.0.1:18000` (proxy) ไม่เปิดให้เครื่องอื่นเข้า

คำสั่งทุกบรรทัดรันบน **Mac** ที่ root ของ repo `/Users/kit/kku/kku-order` ยกเว้นที่มีหมายเหตุ "บน VM"

## a. คัดลอกซอร์ส thaidoc ไป VM

```sh
SCRATCH=$(mktemp -d)
git clone --depth 1 https://github.com/First364600/document-extraction-project-ai "$SCRATCH/thaidoc-src"
git -C "$SCRATCH/thaidoc-src" rev-parse --short HEAD
```

ต้องได้ `37b9975` ถ้าไม่ใช่ ให้หยุดและแจ้งก่อน จากนั้นคัดลอกไป VM:

```sh
ssh project@10.198.200.62 'mkdir -p ~/docflow'
rsync -a --delete --exclude .git "$SCRATCH/thaidoc-src/" project@10.198.200.62:~/docflow/thaidoc-src/
```

## b. คัดลอกโฟลเดอร์ deploy/ocr ไป VM

```sh
ssh project@10.198.200.62 'mkdir -p ~/docflow/kku-order/deploy/ocr'
rsync -a deploy/ocr/ project@10.198.200.62:~/docflow/kku-order/deploy/ocr/
```

อย่าสร้างไฟล์ `.env` บน Mac ค่าจริงจะอยู่บน VM เท่านั้น

## c. บน VM: สร้างไฟล์ .env

บน VM ให้รันตามนี้ (แทน `<hash>` ด้วย hash จากขั้น a ซึ่งคือ `37b9975`):

```sh
cd ~/docflow/kku-order/deploy/ocr
cp .env.example .env
sed -i "s/^OCR_API_KEY=.*/OCR_API_KEY=$(openssl rand -hex 32)/" .env
sed -i 's/^THAIDOC_VERSION=.*/THAIDOC_VERSION=<hash>/' .env
chmod 600 .env
grep -E '^(THAIDOC_VERSION|OCR_PROXY_PUBLISH)=' .env
```

คีย์ `OCR_API_KEY` อยู่ในไฟล์ `.env` อย่าวางในแชทหรือ commit

## d. บน VM: build และรันบริการ

การ build ครั้งแรกใช้เวลา 10 นาทีขึ้นไป จึงรันเป็นพื้นหลังแล้วดู log:

```sh
cd ~/docflow/kku-order/deploy/ocr
nohup docker compose up -d --build > build.log 2>&1 &
tail -f build.log        # กด Ctrl+C เพื่อออกจากการดู log (build ยังทำงานต่อ)
```

เมื่อ build เสร็จ ตรวจสถานะ รอจนแถว `proxy` เป็น `running` และ `thaidoc` เป็น `healthy` (โมเดลโหลดราว 6 วินาที):

```sh
docker compose ps
```

## e. บน VM: ทดสอบ

ทดสอบด้วย curl ที่ `127.0.0.1:18000`

```sh
# 1) ไม่ใส่คีย์ ต้องได้ 401
curl -s -w '\n%{http_code}\n' http://127.0.0.1:18000/health

# 2) ใส่คีย์ /health ต้องได้ 200 และ "models_loaded": true
KEY=$(grep '^OCR_API_KEY=' ~/docflow/kku-order/deploy/ocr/.env | cut -d= -f2-)
curl -s -w '\n%{http_code}\n' -H "Authorization: Bearer $KEY" http://127.0.0.1:18000/health

# 3) /docs ต้องได้ 404 (ทางอื่นทั้งหมดถูกซ่อน)
curl -s -w '\n%{http_code}\n' -H "Authorization: Bearer $KEY" http://127.0.0.1:18000/docs

# 4) ส่ง PDF ตัวอย่าง ต้องได้ 200 และ JSON
#    ไฟล์ตัวอย่างต้องมีอยู่บน VM ที่ ~/kku-order-ocr/docs-samples/kku.pdf ก่อน
mkdir -p ~/kku-order-ocr/out
time curl -s -o ~/kku-order-ocr/out/kku.json -w '%{http_code}\n' \
  -H "Authorization: Bearer $KEY" \
  -F file=@$HOME/kku-order-ocr/docs-samples/kku.pdf \
  http://127.0.0.1:18000/api/v1/extract-json

# สรุปผลจาก JSON
python3 -c 'import json,sys; d=json.load(open(sys.argv[1], encoding="utf-8")); print("number:", d["number"]); print("subject:", d["subject"]); print("date:", d["date"]); print("signer:", d["signer"]); print("groups:", len(d["data"]), "members:", sum(len(g["list name"]) for g in d["data"]))' ~/kku-order-ocr/out/kku.json

# 5) ไฟล์ที่ไม่ใช่ PDF ต้องได้ 400
echo "not a pdf" > ~/kku-order-ocr/out/not-a-pdf.txt
curl -s -w '\n%{http_code}\n' -H "Authorization: Bearer $KEY" \
  -F file=@$HOME/kku-order-ocr/out/not-a-pdf.txt \
  http://127.0.0.1:18000/api/v1/extract-json
```

## f. ต่อ DocFlow backend ภายหลัง

proxy มี alias `ocr-proxy` บน network `docflow-ocr` ตั้งค่าใน compose ของ DocFlow ดังนี้:

```yaml
services:
  api:
    environment:
      OCR_API_URL: http://ocr-proxy:8000
      OCR_API_KEY: <คีย์เดียวกับใน deploy/ocr/.env>
    networks:
      - default
      - docflow-ocr

networks:
  docflow-ocr:
    external: true
```

network `docflow-ocr` ถูกสร้างตอนรัน `docker compose up` ในโฟลเดอร์นี้ ต้องรันขั้น d ก่อน

## g. อัปเดต, rollback, log, หยุดบริการ

อัปเดต thaidoc เป็นเวอร์ชันใหม่ (บน Mac ทำขั้น a ใหม่ แล้ว rsync ซ้ำ จากนั้นบน VM):

```sh
# บน VM: แก้ THAIDOC_VERSION ใน .env เป็น hash ใหม่ แล้ว build
cd ~/docflow/kku-order/deploy/ocr
sed -i 's/^THAIDOC_VERSION=.*/THAIDOC_VERSION=<hash ใหม่>/' .env
nohup docker compose up -d --build > build.log 2>&1 &
```

image ของเวอร์ชันเก่ายังอยู่ใน Docker ดูด้วย `docker images thaidoc-extract`
rollback ทำโดยตั้ง `THAIDOC_VERSION` กลับเป็น hash เก่า แล้วรัน `docker compose up -d` (ไม่ต้อง `--build`)

ดู log:

```sh
docker compose logs -f thaidoc proxy
```

หยุดบริการ (volume ยังอยู่):

```sh
docker compose down
```

อย่าใช้ `docker compose down -v` เพราะจะลบ volume `caddy_data` ด้วย

## h. ย้ายไปเครื่อง GPU ภายหลัง (หมายเหตุสั้น)

- `THAIDOC_DOCKERFILE=Dockerfile.gpu`
- เพิ่ม GPU device reservation ให้ service `thaidoc` ใน `compose.yml` (เช่น `deploy.resources.reservations.devices`)
- `OCR_SITE_ADDRESS=<โดเมน>` เพื่อให้ Caddy ทำ HTTPS เอง
- เปลี่ยน `OCR_PROXY_PUBLISH` เป็น `:80` และ `:443` (ใน compose ต้องเพิ่มพอร์ต 443 และ volume ของ certificate)
- DocFlow เปลี่ยน `OCR_API_URL` เป็น URL ของเครื่อง GPU

## i. เชื่อมแอป DocFlow บน VM เดียวกัน

หลังรันขั้น d แล้ว ให้ตั้งค่าแอป DocFlow บน VM ที่ `~/docflow/kku-order/.env` ดังนี้:

```dotenv
MYSQL_HOST_PORT=127.0.0.1:13307
API_HOST_PORT=127.0.0.1:18080
WEB_HOST_PORT=3000
WEB_ORIGIN=http://10.198.200.62:3000
OCR_API_URL=http://ocr-proxy:8000
OCR_API_KEY=<คัดลอกค่า OCR_API_KEY จาก deploy/ocr/.env>
```

คัดลอกค่าคีย์มาจากไฟล์ `deploy/ocr/.env` โดยตรง อย่าวางในแชทหรือ commit จากนั้นเปิดแอปด้วยคำสั่ง:

```sh
cd ~/docflow/kku-order
docker compose -f docker-compose.yml -f docker-compose.ocr-net.yml up -d --build
```

ไฟล์ `docker-compose.ocr-net.yml` ต่อ service `api` เข้ากับ network `docflow-ocr` เพื่อให้เรียก `ocr-proxy:8000` ได้ network นี้ถูกสร้างตอนรัน `docker compose up` ในโฟลเดอร์ `deploy/ocr/` (ขั้น d)

## j. deploy แอป DocFlow ขึ้น VM (image)

VM ไม่ build โค้ดเอง ให้ build image บน Mac (ชิป Apple Silicon ใช้ buildx สร้าง `linux/amd64`) แล้วส่งขึ้น VM ด้วยคำสั่งเดียว รันจาก root ของ repo และ commit โค้ดก่อน (สคริปต์จะไม่ทำงานถ้ามีไฟล์ที่ติดตามอยู่แก้ไขค้างไว้ เพื่อให้ tag ตรงกับโค้ด):

```sh
scripts/deploy-vm.sh
```

สคริปต์ทำตามลำดับนี้:

1. build `docflow-api:<tag>` และ `docflow-web:<tag>` โดย `<tag>` คือ commit hash สั้น ๆ
2. ส่ง image ทั้งสองไป VM ด้วย `docker save | gzip | ssh ... docker load`
3. คัดลอก `docker-compose.yml` และ `docker-compose.ocr-net.yml` ไปที่ `~/docflow/kku-order` (ไม่แตะ `.env`, `.env.local` และ `docker-compose.vm.yml`)
4. ตั้ง `API_IMAGE` และ `WEB_IMAGE` ใน `.env` ของ VM แล้วรัน `docker compose up -d --no-build api web`
5. รอไม่เกิน 3 นาทีจนบริการ `api` เป็น healthy แล้วแสดง `docker compose ps` ถ้าไม่ผ่าน จะแสดง log 30 บรรทัดล่าสุดของ `api` และจบด้วยสถานะผิดพลาด

ตั้งค่า `VM` หรือ `VM_DIR` ได้ถ้าต้องการปลายทางอื่น เช่น `VM=user@host scripts/deploy-vm.sh`
