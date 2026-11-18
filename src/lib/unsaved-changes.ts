import type { Appointment, ExtractedField } from "./api.ts";
import { guessFor } from "./member-review.ts";

export const leaveMessage = "มีข้อมูลที่แก้ไขแต่ยังไม่ได้บันทึก ถ้าออกจากหน้านี้ข้อมูลจะหายไป ต้องการออกหรือไม่?";

export type LinkClick = {
  href: string | null;
  target?: string | null;
  download?: boolean;
  button: number;
  metaKey: boolean;
  ctrlKey: boolean;
  shiftKey: boolean;
  altKey: boolean;
  defaultPrevented: boolean;
};

// คลิกลิงก์ในระบบที่จะพาแท็บนี้ไปหน้าอื่น ลิงก์ไปเว็บอื่นปล่อยให้เบราว์เซอร์ถามเองผ่าน beforeunload
export function leavesPage(click: LinkClick, current: string): boolean {
  if (!click.href || click.defaultPrevented || click.download) return false;
  if (click.button !== 0 || click.metaKey || click.ctrlKey || click.shiftKey || click.altKey) return false;
  if (click.target && click.target !== "_self") return false;
  const from = new URL(current);
  const to = new URL(click.href, from);
  if (to.origin !== from.origin) return false;
  return to.pathname !== from.pathname || to.search !== from.search;
}

// เหตุการณ์คลิกเท่าที่ตัวกั้นใช้ ใน DOM คือ MouseEvent จริง ในเทสต์เป็นออบเจ็กต์ธรรมดา
export type HeldClick = { preventDefault(): void };

export type LeaveGuardDeps = {
  isDirty: () => boolean;
  currentHref: () => string;
  confirm: (message: string) => boolean;
};

// ตัวกั้นการออกจากหน้าแบบสองจังหวะ
// next/link ทั้งเรียก onClick ของคอมโพเนนต์และสั่งเปลี่ยนหน้าภายในฟังก์ชันเดียว และ React ฟังคลิกที่ document
// จึงไม่มีจังหวะไหนที่ handler ของคอมโพเนนต์ทำงานไปแล้วแต่ Link ยังไม่เปลี่ยนหน้า ตัวกั้นจึงต้อง
// 1) capture: พักการเปลี่ยนหน้าไว้ก่อน (preventDefault ทำให้ Link และเบราว์เซอร์ไม่เปลี่ยนหน้าเอง)
//    แล้วจดว่ามีใครเรียก preventDefault ตามมาหรือไม่ ซึ่งต้องเป็น handler ของหน้าเว็บ เพราะ Link ที่เห็น defaultPrevented จะไม่เรียกซ้ำ
// 2) settle (หลัง handler ทุกตัวทำงานแล้ว): ถ้าหน้าเว็บกันคลิกนี้เอง (เช่น เปิดหน้าต่าง) ก็ไม่ถามและไม่เปลี่ยนหน้า
//    ไม่อย่างนั้นถามผู้ใช้ ตกลงแล้วจึงพาไปตามลิงก์เดิมผ่าน follow ยกเลิกก็อยู่หน้าเดิม โดยไม่หยุดการกระจายเหตุการณ์ของใคร
export function createLeaveGuard(deps: LeaveGuardDeps) {
  const held = new WeakMap<HeldClick, { follow: () => void; preventedByPage: boolean }>();
  return {
    capture(event: HeldClick, click: LinkClick, follow: () => void) {
      if (!deps.isDirty() || !leavesPage(click, deps.currentHref())) return;
      const hold = { follow, preventedByPage: false };
      const preventDefault = event.preventDefault.bind(event);
      preventDefault();
      event.preventDefault = () => {
        hold.preventedByPage = true;
        preventDefault();
      };
      held.set(event, hold);
    },
    settle(event: HeldClick) {
      const hold = held.get(event);
      if (!hold) return;
      held.delete(event);
      if (hold.preventedByPage) return;
      if (deps.isDirty() && !deps.confirm(leaveMessage)) return;
      hold.follow();
    },
  };
}

export type ReviewState = {
  fields: ExtractedField[];
  appointments: Appointment[];
  responsibilities: string;
  additionalReferences: string;
  // แถวที่ผู้ตรวจปฏิเสธชื่อที่ระบบเดา ตอนบันทึกจะไม่ผูกคนที่เดาไว้
  rejectedGuesses: ReadonlySet<number>;
};

// คนที่จะถูกผูกเมื่อกดบันทึก ตรงกับ directoryUserId ใน buildPayload ของหน้าตรวจเอกสาร
function linkedOnSave(row: Appointment, rejectedGuesses: ReadonlySet<number>): number | null {
  return row.directoryUserId ?? (rejectedGuesses.has(row.id) ? undefined : guessFor(row)?.userId) ?? null;
}

// เทียบเฉพาะค่าที่ส่งไปบันทึก รายละเอียดที่ใช้แสดงผลอย่างเดียว เช่น candidates ไม่นับ
function snapshot(state: ReviewState): string {
  return JSON.stringify({
    fields: state.fields.map((field) => [field.id, field.value, field.verified]),
    appointments: state.appointments.map((row) => [row.id, row.fullName, row.position, row.department, row.committeeRole, row.responsibilities, row.extraDuties ?? "", linkedOnSave(row, state.rejectedGuesses), row.nameMatchMethod ?? "", row.verified]),
    responsibilities: state.responsibilities ?? "",
    additionalReferences: state.additionalReferences ?? "",
  });
}

export function reviewChanged(saved: ReviewState, current: ReviewState): boolean {
  return snapshot(saved) !== snapshot(current);
}

export type DutiesDraft = { extra: string; editing: boolean; draft: string; duty: string };

// ข้อความในแท็บหน้าที่ที่พิมพ์ไว้แต่ยังไม่ได้กดเพิ่มหน้าที่หรือใช้ข้อความนี้ ยังไม่อยู่ในข้อมูลที่จะบันทึก ออกจากหน้าแล้วหาย
export function dutiesPending(state: DutiesDraft): boolean {
  return state.extra.trim() !== "" || (state.editing && state.draft !== state.duty);
}
