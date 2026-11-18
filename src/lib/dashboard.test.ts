import { test } from "node:test";
import assert from "node:assert/strict";
import { awaitingConfirmationQuery, loadPanels } from "./dashboard.ts";

test("one failing panel does not blank the others", async () => {
  const panels = await loadPanels({
    summary: async () => ({ total: 12 }),
    documents: async () => { throw new Error("โหลดเอกสารไม่สำเร็จ (500)"); },
    notifications: async () => ["notice"],
  });
  assert.deepEqual(panels.summary, { data: { total: 12 }, error: "" });
  assert.deepEqual(panels.documents, { data: null, error: "โหลดเอกสารไม่สำเร็จ (500)" });
  assert.deepEqual(panels.notifications, { data: ["notice"], error: "" });
});

test("a rejection without an Error still gets a message", async () => {
  const panels = await loadPanels({ summary: () => Promise.reject("nope") });
  assert.deepEqual(panels.summary, { data: null, error: "โหลดข้อมูลไม่สำเร็จ" });
});

// รายการ "รอยืนยันล่าสุด" ต้องใช้กลุ่มเดียวกับตัวเลข "รอยืนยัน" ที่ backend ไม่นับเอกสารที่ OCR ล้มเหลว
test("the awaiting-confirmation panel asks the backend for the same group as the count", () => {
  assert.deepEqual(awaitingConfirmationQuery, { status: "AWAITING_CONFIRMATION", limit: 5 });
});
