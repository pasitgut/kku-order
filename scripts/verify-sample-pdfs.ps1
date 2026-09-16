$ErrorActionPreference = "Stop"

$apiBase = "http://localhost:8080"
$sampleFiles = @(
    (Join-Path $PSScriptRoot "..\..\malicious_file__open_with_care.pdf"),
    (Join-Path $PSScriptRoot "..\..\kku.pdf")
)

try {
    $health = Invoke-RestMethod -Uri "$apiBase/health" -TimeoutSec 5
    if ($health.status -ne "ok") { throw "API ยังไม่ healthy" }
} catch {
    throw "เชื่อมต่อ API ไม่ได้ กรุณารัน docker compose ก่อน: $($_.Exception.Message)"
}

foreach ($path in $sampleFiles) {
    $resolved = [IO.Path]::GetFullPath($path)
    if (-not (Test-Path -LiteralPath $resolved -PathType Leaf)) {
        throw "ไม่พบไฟล์ตัวอย่าง: $resolved"
    }

    Write-Output "UPLOAD=$resolved"
    $response = Invoke-RestMethod -Uri "$apiBase/api/v1/documents" -Method Post -Form @{
        file = Get-Item -LiteralPath $resolved
        overwrite = "true"
    }
    $documentId = [int]$response.data.id
    Write-Output "DOCUMENT_ID=$documentId"

    $deadline = (Get-Date).AddMinutes(20)
    do {
        Start-Sleep -Seconds 5
        $document = (Invoke-RestMethod -Uri "$apiBase/api/v1/documents/$documentId" -TimeoutSec 10).data
        Write-Output "STATUS=$($document.status) STAGE=$($document.processingStage)"
        if ($document.status -in @("REVIEW", "NEEDS_REVIEW", "FAILED")) { break }
    } while ((Get-Date) -lt $deadline)

    if ($document.status -notin @("REVIEW", "NEEDS_REVIEW")) {
        throw "ประมวลผลเอกสาร $documentId ไม่เสร็จภายใน 20 นาที: $($document.status) / $($document.ocrError)"
    }

    Write-Output "RESULT=$($document.originalFilename) PAGES=$($document.pageCount) APPOINTMENTS=$($document.personCount) FIELDS=$($document.fields.Count) OCR=$($document.ocrProvider)"
}

Write-Output "SAMPLE_PDFS_OK"
