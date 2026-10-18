export type Period = "all" | "this" | "last" | "custom";
export type RecentSearch = { kind: "person" | "order"; id: number; label: string };
export type AppointmentState = "ACTIVE" | "EXPIRING" | "EXPIRED" | "CANCELLED";

const recentLimit = 6;

function isoDate(date: Date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

// fiscalYearRange ปีงบประมาณเริ่ม 1 ต.ค. จบ 30 ก.ย. เรียกตามปี พ.ศ. ที่จบ (offset -1 คือปีงบก่อน)
export function fiscalYearRange(today: Date, offset = 0) {
  const endYear = today.getFullYear() + (today.getMonth() >= 9 ? 1 : 0) + offset;
  return { from: isoDate(new Date(endYear - 1, 9, 1)), to: isoDate(new Date(endYear, 8, 30)), label: `ปีงบ ${endYear + 543}` };
}

// periodRange แปลงช่วงเวลาที่เลือกเป็นช่วงวันที่ออกคำสั่ง ช่องที่ว่างไม่ส่ง
export function periodRange(period: Period, today: Date, custom: { from: string; to: string } = { from: "", to: "" }): { from?: string; to?: string } {
  if (period === "all") return {};
  if (period === "custom") return Object.fromEntries(Object.entries(custom).filter(([, value]) => value));
  const { from, to } = fiscalYearRange(today, period === "last" ? -1 : 0);
  return { from, to };
}

// rememberSearch ใส่รายการล่าสุดไว้หน้าสุด ไม่ซ้ำ และเก็บไม่เกิน 6 รายการ
export function rememberSearch(list: RecentSearch[], entry: RecentSearch) {
  return [entry, ...list.filter((item) => item.kind !== entry.kind || item.id !== entry.id)].slice(0, recentLimit);
}

// moveSelection เลื่อนแถวที่เลือกด้วยลูกศร วนจากท้ายกลับไปต้น
export function moveSelection(current: number, delta: number, length: number) {
  if (length === 0) return -1;
  if (current < 0) return delta > 0 ? 0 : length - 1;
  return (current + delta + length) % length;
}

const stateLabels: Record<AppointmentState, string> = { ACTIVE: "มีผล", EXPIRING: "ใกล้หมดวาระ", EXPIRED: "หมดวาระ", CANCELLED: "ยกเลิก" };

export function appointmentStateLabel(state: AppointmentState) {
  return stateLabels[state] ?? state;
}
