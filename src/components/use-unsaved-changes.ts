"use client";

import { useEffect } from "react";
import { createLeaveGuard } from "@/lib/unsaved-changes";

// ทุกฟอร์มที่มีข้อมูลค้างใช้ตัวดักชุดเดียวกัน หน้าที่มีหลายฟอร์มจึงถามผู้ใช้แค่ครั้งเดียว
const dirtyOwners = new Set<symbol>();
let leaveAllowedUntil = 0;

// เรียกก่อนโหลดหน้าใหม่เอง (เช่น หลังส่งไฟล์ทับ) เพื่อไม่ให้เบราว์เซอร์ถามซ้ำ
export function allowLeave() {
  leaveAllowedUntil = Date.now() + 2000;
}

function onBeforeUnload(event: BeforeUnloadEvent) {
  if (!dirtyOwners.size || Date.now() < leaveAllowedUntil) return;
  event.preventDefault();
  event.returnValue = "";
}

const guard = createLeaveGuard({ isDirty: () => dirtyOwners.size > 0, currentHref: () => window.location.href, confirm: (message) => window.confirm(message) });
let following = false;
let clicksWired = false;

// ลิงก์ของ Next ไม่ทำให้เกิด beforeunload จึงดักคลิกเอง ลำดับของ listener ที่ document (ราก React ของ App Router คือ document):
// capture ของเรา -> handler ของคอมโพเนนต์และ next/link (React ฟังแบบ bubble ที่ document และลงทะเบียนไว้ก่อนเรา) -> bubble ของเรา
// capture พักการเปลี่ยนหน้าไว้ bubble จึงรู้แล้วว่าหน้าเว็บกันคลิกนี้เองหรือไม่ ก่อนตัดสินใจถาม (รายละเอียดที่ createLeaveGuard)
function onClickCapture(event: MouseEvent) {
  if (following) return;
  const anchor = event.target instanceof Element ? event.target.closest("a[href]") : null;
  if (!(anchor instanceof HTMLAnchorElement)) return;
  const click = { href: anchor.href, target: anchor.target, download: anchor.hasAttribute("download"), button: event.button, metaKey: event.metaKey, ctrlKey: event.ctrlKey, shiftKey: event.shiftKey, altKey: event.altKey, defaultPrevented: event.defaultPrevented };
  guard.capture(event, click, () => {
    // ผู้ใช้ยืนยันแล้ว คลิกลิงก์เดิมซ้ำให้ next/link หรือเบราว์เซอร์เปลี่ยนหน้าตามปกติ โดยไม่ถามซ้ำ
    allowLeave();
    following = true;
    try {
      anchor.click();
    } finally {
      following = false;
    }
  });
}

// ฟังที่ document ไม่ใช่ window เพื่อให้ทำงานแม้ handler ของ React เรียก stopPropagation (หยุดได้แค่โหนดถัดไป ไม่ใช่ listener บน document เดียวกัน)
function onClickBubble(event: MouseEvent) {
  guard.settle(event);
}

function register(owner: symbol) {
  if (!clicksWired) {
    // ติดตั้งครั้งเดียวและไม่ถอด: ถ้าถอดระหว่าง capture กับ bubble (เช่น handler ล้างฟอร์มจนไม่มีข้อมูลค้าง) คลิกที่พักไว้จะหายไปเฉย ๆ
    document.addEventListener("click", onClickCapture, true);
    document.addEventListener("click", onClickBubble);
    clicksWired = true;
  }
  if (!dirtyOwners.size) window.addEventListener("beforeunload", onBeforeUnload);
  dirtyOwners.add(owner);
}

function unregister(owner: symbol) {
  dirtyOwners.delete(owner);
  if (!dirtyOwners.size) window.removeEventListener("beforeunload", onBeforeUnload);
}

// ถามก่อนออกจากหน้าเมื่อยังมีข้อมูลที่แก้แต่ไม่ได้บันทึก (รีโหลด ปิดแท็บ หรือกดลิงก์ในระบบ)
// ตั้งใจไม่ดักปุ่ม Back ของเบราว์เซอร์ (popstate) ระหว่างหน้าในระบบ: App Router เปลี่ยนหน้าโดยไม่เกิด beforeunload
// และยกเลิก popstate ไม่ได้ ต้องใส่ history sentinel ซึ่งผู้ใช้ตัดสินใจไม่ทำ (QA C15) ปุ่มย้อนกลับใน AppShell เป็นลิงก์จึงยังถูกดัก
export function useUnsavedChanges(dirty: boolean) {
  useEffect(() => {
    if (!dirty) return;
    const owner = Symbol("unsaved-changes");
    register(owner);
    return () => unregister(owner);
  }, [dirty]);
}
