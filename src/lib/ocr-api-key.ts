export type OCRKeySource = "web" | "env" | "none";

// The placeholder shows the current state without ever showing the key itself.
export function ocrAPIKeyPlaceholder(source: OCRKeySource, hint: string): string {
  const tail = hint ? ` ลงท้าย ••••${hint}` : "";
  if (source === "web") return `ตั้งไว้แล้ว${tail} (จากหน้าเว็บ)`;
  if (source === "env") return `ตั้งไว้แล้ว${tail} (จากไฟล์ .env ของเซิร์ฟเวอร์)`;
  return "ยังไม่ได้ตั้ง";
}
