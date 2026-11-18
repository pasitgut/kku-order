export type ProcessingDoc = {
  status: string;
  processingStage?: string;
  extractionAttempts?: number;
  nextAttemptAt?: string | null;
  ocrError?: string;
  confidence: number;
};

const timeFormat = new Intl.DateTimeFormat("en-GB", {
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
  timeZone: "Asia/Bangkok",
});

function retryTime(value?: string | null): string {
  const when = value ? new Date(value) : null;
  return when && !Number.isNaN(when.getTime()) ? timeFormat.format(when) : "-";
}

// n คือครั้งที่จะลองถัดไป จึงบวก 1 จากจำนวนครั้งที่เริ่มไปแล้ว
// เอกสารที่ยกเลิกแล้วไม่ถูกประมวลผลต่อ แม้ processing_stage จะค้างเป็น RETRY_WAIT หรือ QUEUED
export function processingNote(doc: ProcessingDoc): string | null {
  if (doc.status === "CANCELLED") return null;
  if (doc.processingStage === "RETRY_WAIT") {
    const upcoming = (doc.extractionAttempts ?? 0) + 1;
    const when = retryTime(doc.nextAttemptAt);
    return `รอลองใหม่ ครั้งที่ ${upcoming} เวลา ${when}`;
  }
  if (doc.processingStage === "FAILED") {
    return "ประมวลผลไม่สำเร็จ";
  }
  return null;
}

// เอกสารที่ยังไม่ยกเลิกและไม่มี worker ถืออยู่ สแกนไฟล์เดิมใหม่ได้ (เกณฑ์เดียวกับ backend)
export function canRescan(doc: ProcessingDoc): boolean {
  if (doc.status === "CANCELLED") return false;
  return !["QUEUED", "EXTRACTING", "PREPARING"].includes(doc.processingStage ?? "");
}

// เอกสารที่ล้มเหลวหรือรอลองใหม่ยังไม่เคยได้ผล จึงกดได้ทันที ส่วนเอกสารที่เสร็จแล้วต้องยืนยันสองขั้น
export function rescanNeedsConfirm(doc: ProcessingDoc): boolean {
  return doc.processingStage !== "FAILED" && doc.processingStage !== "RETRY_WAIT";
}

// ระหว่างประมวลผล confidence 0 หมายถึงกำลังคำนวณ; เสร็จแล้วยัง 0 หมายถึงไม่ระบุ
export function confidenceLabel(doc: ProcessingDoc): string {
  if (doc.status === "PROCESSING") return "กำลังคำนวณ";
  if (doc.confidence === 0) return "ไม่ระบุ";
  return `${Math.round(doc.confidence * 100)}%`;
}

export function isInsecureURL(url: string): boolean {
  return url.trim().toLowerCase().startsWith("http://");
}
