export type ReportFormat = "pdf" | "docx" | "xlsx";
export type ReportFilters = { person?: number; from?: string; to?: string; activeOnly?: boolean };

export const reportFormats: { format: ReportFormat; label: string }[] = [
  { format: "pdf", label: "PDF" },
  { format: "docx", label: "Word" },
  { format: "xlsx", label: "Excel" },
];

// reportQuery ส่งเฉพาะตัวกรองที่ตั้งไว้ ให้รายงานตรงกับที่เห็นบนหน้าค้นหา
export function reportQuery(format: ReportFormat, filters: ReportFilters) {
  const params = new URLSearchParams({ format });
  if (filters.person) params.set("person", String(filters.person));
  if (filters.from) params.set("from", filters.from);
  if (filters.to) params.set("to", filters.to);
  if (filters.activeOnly) params.set("activeOnly", "1");
  return params.toString();
}

// filenameFromDisposition อ่านชื่อไฟล์ภาษาไทยจาก filename* ก่อน ถ้าไม่มีใช้ filename ธรรมดา
export function filenameFromDisposition(header: string | null, fallback: string) {
  if (!header) return fallback;
  const encoded = /filename\*=UTF-8''([^;]+)/i.exec(header);
  if (encoded) {
    try { return decodeURIComponent(encoded[1]); } catch { /* ใช้ชื่อสำรอง */ }
  }
  const plain = /filename="([^"]+)"/i.exec(header);
  return plain ? plain[1] : fallback;
}
