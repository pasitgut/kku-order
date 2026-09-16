FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/docflow-api .

FROM python:3.12-slim
WORKDIR /app
RUN apt-get update \
    && apt-get install -y --no-install-recommends poppler-utils \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system app \
    && useradd --system --gid app app
COPY --from=builder /out/docflow-api /app/docflow-api
COPY ocr_worker.py /app/ocr_worker.py
COPY one-ocr /app/one-ocr
RUN pip install --no-cache-dir -e /app/one-ocr \
    && mkdir -p /app/storage \
    && chown -R app:app /app
USER app
EXPOSE 8080
ENV OCR_SCRIPT=/app/ocr_worker.py \
    OCR_MODELS_DIR=/app/one-ocr/models \
    OCR_PYTHON=python \
    PDFTOPPM_BIN=pdftoppm \
    PDFINFO_BIN=pdfinfo
CMD ["/app/docflow-api"]
