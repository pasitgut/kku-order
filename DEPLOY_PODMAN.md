# DocFlow: Build Docker, ส่ง image ด้วย SCP และ Deploy ด้วย Podman

เอกสารนี้ใช้เมื่อเครื่อง build มี Docker แต่ server ปลายทางไม่มี Docker และใช้ Podman แทน โดย server มี Cloudflared Tunnel ที่เชื่อมกับ network ชื่อ proxy-net

สมมติฐาน:

- Oracle VM เป็น Linux ARM64 ดังนั้น image ทุกตัวต้อง build เป็น linux/arm64
- server ติดตั้ง podman และ podman-compose หรือ podman compose
- Cloudflared ทำงานเป็น container และอยู่บน proxy-net
- ใช้ชื่อ service api, mysql และ docflow-web ตาม production compose

## 1. โครงสร้าง production

| Container | Image | Network | หน้าที่ |
|---|---|---|---|
| docflow-mysql | mysql:8.4 | docflow-internal | MySQL และ migration data |
| docflow-api | docflow-api:<tag> | docflow-internal | Go Gin/GORM, OCR, cron sync |
| docflow-web | docflow-web:<tag> | docflow-internal, proxy-net | Next.js และ proxy /backend/* |

Cloudflared ต้องเรียก Web ผ่าน:

~~~text
http://docflow-web:3000
~~~

ใช้ไฟล์ production compose ที่ deploy/podman-compose.yml ซึ่งแยกจาก docker-compose.yml สำหรับ local development

## 2. Build image บนเครื่อง Windows

เปิด PowerShell แล้วเข้าโปรเจกต์:

~~~powershell
cd C:\Users\T.Home\Documents\projects\university\document-command-center
docker version
docker buildx version
ssh -V
scp -V
~~~

### 2.1 ตรวจสอบ OneOCR models

API image ต้องมี OneOCR models ไม่เช่นนั้น build อาจผ่าน แต่ OCR จะประมวลผลเอกสารไม่ได้:

~~~powershell
Test-Path backend\one-ocr\models\detector\text_detector.onnx
Test-Path backend\one-ocr\models\recognizers
Test-Path backend\one-ocr\models\vocab
~~~

ถ้าไม่มี ให้เตรียม models ตาม backend/one-ocr/README.md ก่อน build ใหม่ โดยไม่ต้อง commit models เข้า Git

### 2.2 กำหนด release tag

~~~powershell
$Tag = Get-Date -Format "yyyyMMdd-HHmmss"
$ApiImage = "docflow-api:$Tag"
$WebImage = "docflow-web:$Tag"
$Bundle = "docflow-images-$Tag.tar"
$Tag
~~~

ถ้าต้องการกำหนด tag เอง:

~~~powershell
$Tag = "20260919-01"
$ApiImage = "docflow-api:$Tag"
$WebImage = "docflow-web:$Tag"
$Bundle = "docflow-images-$Tag.tar"
~~~

### 2.3 เตรียม buildx

~~~powershell
docker buildx create --name docflow-builder --use
docker buildx inspect --bootstrap
~~~

ถ้ามี builder อยู่แล้ว:

~~~powershell
docker buildx use docflow-builder
docker buildx inspect --bootstrap
~~~

ถ้าเครื่อง build เป็น Windows x64 ไม่เป็นปัญหา เพราะ Docker Buildx จะ build image สำหรับ ARM64 ผ่าน emulation แต่อาจใช้เวลานานกว่าปกติ ให้ตรวจว่า Docker Desktop เปิดใช้งาน Linux containers แล้ว

### 2.4 Build API และ Web

~~~powershell
docker buildx build --platform linux/arm64 --load --tag $ApiImage ./backend
docker buildx build --platform linux/arm64 --load --build-arg NEXT_PUBLIC_API_URL=/backend --build-arg BACKEND_INTERNAL_URL=http://api:8080 --tag $WebImage .
docker pull --platform linux/arm64 mysql:8.4
docker image inspect $ApiImage --format '{{.Os}}/{{.Architecture}}'
docker image inspect $WebImage --format '{{.Os}}/{{.Architecture}}'
docker image inspect mysql:8.4 --format '{{.Os}}/{{.Architecture}}'
docker images docflow-api
docker images docflow-web
docker images mysql
~~~

BACKEND_INTERNAL_URL ต้องเป็น http://api:8080 เพราะ Web จะเรียก API ผ่าน network ภายใน Podman

ห้ามใส่ API_KEY, .env.local หรือ secret อื่นลงใน image

## 3. ทดสอบก่อนส่งขึ้น server

~~~powershell
docker compose --env-file .env.local up -d --build
docker compose ps
Invoke-WebRequest http://localhost:8080/health
docker compose down
git check-ignore .env.local
git check-ignore backend\.env.local
~~~

## 4. Save image เป็น .tar

รวม API, Web และ MySQL เป็น bundle เดียว:

~~~powershell
docker save --platform linux/arm64 --output $Bundle $ApiImage $WebImage mysql:8.4
Get-Item $Bundle
Get-FileHash $Bundle -Algorithm SHA256 | Tee-Object "$Bundle.sha256"
~~~

ถ้ายังพบ `content digest ... not found` ให้ rebuild image โดยปิด provenance/SBOM ที่อาจทำให้ image store ของ Docker Desktop รุ่นเก่าหรือ containerd store export ไม่ครบ แล้วลอง save ใหม่:

~~~powershell
docker buildx build --no-cache --provenance=false --sbom=false --platform linux/arm64 --load --tag $ApiImage ./backend
docker buildx build --no-cache --provenance=false --sbom=false --platform linux/arm64 --load --build-arg NEXT_PUBLIC_API_URL=/backend --build-arg BACKEND_INTERNAL_URL=http://api:8080 --tag $WebImage .
docker save --platform linux/arm64 --output $Bundle $ApiImage $WebImage mysql:8.4
~~~

ตรวจสอบว่า Docker Desktop เป็นเวอร์ชันปัจจุบันและใช้ Linux containers หาก `docker save` ไม่รู้จัก option `--platform`

## 5. ส่ง image และ compose ด้วย SCP

กำหนดค่าการเชื่อมต่อ:

~~~powershell
$ServerUser = "deploy"
$ServerHost = "your-server.example.ac.th"
$RemoteDir = "/opt/docflow/releases/$Tag"
~~~

สร้าง directory บน server และส่งไฟล์:

~~~powershell
ssh "$ServerUser@$ServerHost" "sudo mkdir -p $RemoteDir /opt/docflow"
scp $Bundle "$($ServerUser)@$($ServerHost):$RemoteDir/"
scp "$Bundle.sha256" "$($ServerUser)@$($ServerHost):$RemoteDir/"
scp deploy/podman-compose.yml "$($ServerUser)@$($ServerHost):/opt/docflow/podman-compose.yml"
~~~

ถ้า SSH ใช้ port อื่น ให้เพิ่ม -P <port> ในทั้ง ssh และ scp

## 6. เตรียม Podman บน server

~~~powershell
ssh "$ServerUser@$ServerHost"
~~~

ตรวจสอบ:

~~~bash
podman --version
podman info
podman-compose --version
~~~

กำหนดตัวแปร compose ตามที่มีใน server:

~~~bash
COMPOSE="podman-compose"
~~~

หรือถ้า server ใช้ plugin:

~~~bash
COMPOSE="podman compose"
~~~

ต้องใช้ mode เดียวกับ Cloudflared เสมอ ถ้า Cloudflared รันด้วย sudo podman ให้ใช้ sudo podman และ sudo podman-compose ทุกคำสั่ง เพราะ rootless/rootful มี network แยกกัน

## 7. เตรียม proxy-net และ Cloudflared

~~~bash
podman network ls
podman network inspect proxy-net
~~~

ถ้ายังไม่มี network:

~~~bash
podman network create proxy-net
~~~

ตรวจชื่อ Cloudflared container:

~~~bash
podman ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
~~~

ถ้าชื่อ container คือ cloudflared และยังไม่ได้อยู่ใน network:

~~~bash
podman network connect proxy-net cloudflared
podman network inspect proxy-net
~~~

ถ้า Cloudflared เป็น system service บน host ไม่ใช่ container จะใช้ proxy-net ไม่ได้โดยตรง ให้ใช้วิธี host port ในหัวข้อ Troubleshooting แทน

## 8. โหลด image จาก .tar

~~~bash
export TAG=20260919-01
export RELEASE_DIR=/opt/docflow/releases/$TAG
cd "$RELEASE_DIR"
sha256sum -c "docflow-images-$TAG.tar.sha256"
podman load --input "docflow-images-$TAG.tar"
podman images | grep -E 'docflow-api|docflow-web|mysql'
~~~

## 9. สร้าง environment production

สร้าง /opt/docflow/.env บน server:

~~~bash
cd /opt/docflow
vi .env
~~~

ตัวอย่างค่า:

~~~dotenv
IMAGE_TAG=20260919-01

MYSQL_DATABASE=docflow
MYSQL_USER=docflow
MYSQL_PASSWORD=เปลี่ยนรหัสผ่านฐานข้อมูล
MYSQL_ROOT_PASSWORD=เปลี่ยนรหัสผ่าน root
DB_DSN=docflow:เปลี่ยนรหัสผ่านฐานข้อมูล@tcp(mysql:3306)/docflow?charset=utf8mb4&parseTime=True&loc=Local

API_KEY=ใส่คีย์ของระบบภายนอก
EXTERNAL_USERS_URL=https://fs.computing.kku.ac.th/api/ext/v1/users
USER_SYNC_CRON=0 5 * * *
USER_SYNC_TZ=Asia/Bangkok
EXPIRY_CRON=10 5 * * *
IMPORT_CRON=*/5 * * *

AUTH_REQUIRED=true
AUTH_USER_HEADER=X-Auth-Request-User
AUTH_ROLE_HEADER=X-Auth-Request-Role
WEB_ORIGIN=https://ชื่อโดเมนของระบบ

OCR_LANGUAGE=thai
OCR_DPI=200
OCR_MAX_SIDE=1536
OCR_PREPROCESS=none
~~~

ตั้ง permission:

~~~bash
chmod 600 /opt/docflow/.env
~~~

ห้าม commit ไฟล์นี้ และอย่าใส่ API key ใน image bundle

## 10. Deploy ด้วย Podman

ไฟล์ production compose จะสร้าง network ภายใน docflow-internal และต่อ docflow-web เข้า proxy-net:

~~~bash
cd /opt/docflow
$COMPOSE -f podman-compose.yml up -d
$COMPOSE -f podman-compose.yml ps
podman ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
~~~

รอ API พร้อมและตรวจ health จากภายใน container เนื่องจาก production compose ไม่ publish port ออก host:

~~~bash
until podman exec docflow-api python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/health', timeout=3)"; do sleep 2; done
podman exec docflow-api python -c "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8080/health', timeout=3).read().decode())"
podman exec docflow-web wget -qO- http://127.0.0.1:3000
~~~

ตรวจ log และ migration:

~~~bash
podman logs --tail 100 docflow-mysql
podman logs --tail 100 docflow-api
podman logs --tail 100 docflow-web
podman exec docflow-mysql sh -lc 'exec mysql -udocflow -p"$MYSQL_PASSWORD" docflow -e "SELECT version, applied_at FROM schema_migrations ORDER BY version;"'
~~~

## 11. ตั้งค่า Cloudflared Tunnel

ตัวอย่าง config:

~~~yaml
tunnel: <tunnel-id>
credentials-file: /etc/cloudflared/<tunnel-id>.json

ingress:
  - hostname: docs.example.ac.th
    service: http://docflow-web:3000
  - service: http_status:404
~~~

หลังแก้ config:

~~~bash
podman restart cloudflared
podman logs --tail 100 cloudflared
podman network inspect proxy-net
~~~

ทดสอบจากภายนอก:

~~~bash
curl -I https://docs.example.ac.th
curl -fsS https://docs.example.ac.th/backend/health
~~~

## 12. สั่ง sync รายชื่อครั้งแรก

ระบบจะ sync อัตโนมัติทุกวันเวลา 05:00 น. ตาม Asia/Bangkok แต่หลัง deploy ควรสั่งครั้งแรกเอง:

~~~bash
curl -X POST https://docs.example.ac.th/backend/api/v1/sync/users \
  -H 'X-Auth-Request-User: deploy-admin' \
  -H 'X-Auth-Request-Role: ADMIN'

curl -fsS https://docs.example.ac.th/backend/api/v1/sync/runs \
  -H 'X-Auth-Request-User: deploy-admin' \
  -H 'X-Auth-Request-Role: ADMIN'
~~~

## 13. อัปเดต release รอบถัดไป

บนเครื่อง build ให้สร้าง tag ใหม่ แล้วทำ build/save/scp ซ้ำ เช่น 20260920-01

บน server:

~~~bash
export TAG=20260920-01
export RELEASE_DIR=/opt/docflow/releases/$TAG
cd "$RELEASE_DIR"
sha256sum -c "docflow-images-$TAG.tar.sha256"
podman load --input "docflow-images-$TAG.tar"
~~~

แก้ IMAGE_TAG ใน /opt/docflow/.env แล้ว deploy ใหม่:

~~~bash
vi /opt/docflow/.env
cd /opt/docflow
$COMPOSE -f podman-compose.yml up -d
~~~

ห้ามลบ volume docflow_mysql_data หรือ docflow_uploads ระหว่าง update

## 14. Rollback

~~~bash
podman load --input /opt/docflow/releases/<old-tag>/docflow-images-<old-tag>.tar
vi /opt/docflow/.env
cd /opt/docflow
$COMPOSE -f podman-compose.yml up -d
podman exec docflow-api python -c "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8080/health', timeout=3).read().decode())"
~~~

## 15. Backup

Backup MySQL:

~~~bash
mkdir -p /opt/docflow/backups
podman exec docflow-mysql sh -lc 'exec mysqldump -udocflow -p"$MYSQL_PASSWORD" --single-transaction --routines --events docflow' \
  > "/opt/docflow/backups/docflow-$(date +%Y%m%d-%H%M%S).sql"
~~~

ไฟล์ PDF อยู่ใน volume docflow_uploads ไม่ได้อยู่ใน MySQL จึงต้อง backup volume/พื้นที่เก็บไฟล์ด้วย:

~~~bash
podman volume ls | grep docflow
podman volume inspect docflow_mysql_data
podman volume inspect docflow_uploads
~~~

## 16. Troubleshooting

### Web เรียก API ไม่ได้

~~~bash
podman exec docflow-web getent hosts api
podman exec docflow-web wget -qO- http://api:8080/health
~~~

ตรวจว่า API และ Web อยู่ใน docflow-internal เดียวกัน และ Web ใช้ BACKEND_INTERNAL_URL=http://api:8080

### Cloudflared ขึ้น 502

~~~bash
podman network inspect proxy-net
podman inspect docflow-web --format '{{json .NetworkSettings.Networks}}'
podman inspect cloudflared --format '{{json .NetworkSettings.Networks}}'
podman exec cloudflared getent hosts docflow-web
podman exec cloudflared wget -qO- http://docflow-web:3000
~~~

ถ้าไม่เห็น docflow-web ให้ใช้ Podman mode เดียวกันแล้วเชื่อม network ใหม่:

~~~bash
podman network connect proxy-net cloudflared
podman restart cloudflared
~~~

### API ต่อ MySQL ไม่ได้

~~~bash
podman exec docflow-api getent hosts mysql
podman logs docflow-api
podman logs docflow-mysql
~~~

DB_DSN ของ API ต้องใช้ mysql:3306 ไม่ใช่ localhost:3307

### OCR ไม่พบ models

~~~bash
podman exec docflow-api sh -lc 'find /app/one-ocr/models -maxdepth 3 -type f | head -30'
~~~

ถ้าไม่พบ ให้กลับไปเตรียม backend/one-ocr/models บนเครื่อง build แล้ว build API image ใหม่

### Migration ไม่ทำงาน

~~~bash
podman logs docflow-api | grep -i migration
podman exec docflow-mysql sh -lc 'exec mysql -udocflow -p"$MYSQL_PASSWORD" docflow -e "SELECT * FROM schema_migrations ORDER BY version;"'
podman restart docflow-api
~~~

อย่าลบ database volume เพื่อแก้ migration error

### Cloudflared รันเป็น system service บน host

production compose นี้ไม่ publish Web ออก host ดังนั้น Cloudflared ต้องรันเป็น container และอยู่ใน proxy-net เดียวกับ docflow-web ตามหัวข้อ 7 และ 11 หาก Cloudflared เป็น system service บน host ให้เพิ่ม port ชั่วคราวเฉพาะ Web:

~~~yaml
services:
  web:
    ports:
      - "127.0.0.1:3000:3000"
~~~

แล้วตั้ง ingress เป็น http://127.0.0.1:3000 หรือย้าย Cloudflared มาเป็น container บน proxy-net

## 17. Checklist หลัง deploy

- [ ] checksum ของ image bundle ผ่าน
- [ ] podman load สำเร็จ
- [ ] docflow-mysql, docflow-api, docflow-web ทำงานอยู่
- [ ] API health เป็น 200
- [ ] migration version ล่าสุดอยู่ใน schema_migrations
- [ ] docflow-web และ Cloudflared อยู่ใน proxy-net
- [ ] Cloudflared เรียก http://docflow-web:3000 ได้
- [ ] เปิดหน้าเว็บผ่าน domain จริงได้
- [ ] sync รายชื่อจาก External API สำเร็จ
- [ ] cron ใช้ Asia/Bangkok และเวลา 05:00
- [ ] OCR models อยู่ใน API image
- [ ] มี backup ของ MySQL และไฟล์เอกสาร
- [ ] .env และ API_KEY ไม่อยู่ใน Git หรือ image bundle
