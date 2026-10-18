FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/docflow-api .

FROM python:3.12-slim
WORKDIR /app
# libpango + libharfbuzz-subset ให้ WeasyPrint พิมพ์รายงาน PDF ภาษาไทยได้ถูกต้อง (report_pdf.py)
RUN apt-get update \
    && apt-get install -y --no-install-recommends poppler-utils libpango-1.0-0 libpangoft2-1.0-0 libharfbuzz-subset0 \
    && rm -rf /var/lib/apt/lists/* \
    && pip install --no-cache-dir weasyprint==69.0 \
    && groupadd --system app \
    && useradd --system --gid app app
COPY --from=builder /out/docflow-api /app/docflow-api
COPY ocr_worker.py /app/ocr_worker.py
COPY report_pdf.py /app/report_pdf.py
COPY fonts /app/fonts
RUN mkdir -p /app/storage && chown -R app:app /app
USER app
EXPOSE 8080
ENV OCR_SCRIPT=/app/ocr_worker.py \
    OCR_PYTHON=python \
    PDFTOPPM_BIN=pdftoppm \
    PDFINFO_BIN=pdfinfo \
    REPORT_PDF_SCRIPT=/app/report_pdf.py \
    REPORT_PDF_PYTHON=python \
    REPORT_FONTS_DIR=/app/fonts \
    XDG_CACHE_HOME=/tmp/.cache
CMD ["/app/docflow-api"]
