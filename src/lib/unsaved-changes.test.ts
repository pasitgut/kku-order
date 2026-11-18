import { test } from "node:test";
import assert from "node:assert/strict";
import type { Appointment, ExtractedField } from "./api.ts";
import { createLeaveGuard, dutiesPending, leaveMessage, leavesPage, reviewChanged, type LinkClick } from "./unsaved-changes.ts";

const here = "http://docflow.test/documents/57";
const click = (href: string | null, extra: Partial<LinkClick> = {}): LinkClick => ({ href, target: "", download: false, button: 0, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, defaultPrevented: false, ...extra });

test("a plain click on an in-app link to another page leaves the page", () => {
  assert.equal(leavesPage(click("http://docflow.test/documents"), here), true);
  assert.equal(leavesPage(click("http://docflow.test/documents/57?tab=history"), here), true);
});

test("clicks that keep this tab on the same page do not leave it", () => {
  assert.equal(leavesPage(click("http://docflow.test/documents/57"), here), false);
  assert.equal(leavesPage(click("http://docflow.test/documents/57#roster"), here), false);
  assert.equal(leavesPage(click(null), here), false);
});

test("new-tab, download and modified clicks do not leave the page", () => {
  assert.equal(leavesPage(click("http://docflow.test/documents", { target: "_blank" }), here), false);
  assert.equal(leavesPage(click("http://docflow.test/documents", { download: true }), here), false);
  assert.equal(leavesPage(click("http://docflow.test/documents", { ctrlKey: true }), here), false);
  assert.equal(leavesPage(click("http://docflow.test/documents", { metaKey: true }), here), false);
  assert.equal(leavesPage(click("http://docflow.test/documents", { shiftKey: true }), here), false);
  assert.equal(leavesPage(click("http://docflow.test/documents", { button: 1 }), here), false);
  assert.equal(leavesPage(click("http://docflow.test/documents", { defaultPrevented: true }), here), false);
});

test("links to another site are left to the browser's own leave prompt", () => {
  assert.equal(leavesPage(click("https://drive.google.com/file/d/1"), here), false);
});

test("a link click its own handler already prevented (e.g. one that opens a dialog) does not leave the page", () => {
  assert.equal(leavesPage(click("http://docflow.test/documents/57/history", { defaultPrevented: true }), here), false);
});

// เหตุการณ์คลิกจำลอง preventDefault ทำงานแบบเดียวกับ Event จริง
const fakeEvent = (defaultPrevented = false) => ({ defaultPrevented, preventDefault() { this.defaultPrevented = true; } });

// จำลองลำดับใน DOM: capture ของตัวกั้น -> handler ของคอมโพเนนต์/next/link -> settle ตอน bubble
function setup({ dirty = true, answer = true } = {}) {
  const asked: string[] = [];
  const followed: string[] = [];
  const state = { dirty };
  const guard = createLeaveGuard({ isDirty: () => state.dirty, currentHref: () => here, confirm: (message) => { asked.push(message); return answer; } });
  const press = (link: LinkClick, event = fakeEvent(link.defaultPrevented), handler: (e: { defaultPrevented: boolean; preventDefault(): void }) => void = () => {}) => {
    guard.capture(event, link, () => followed.push(link.href ?? ""));
    handler(event);
    guard.settle(event);
    return event;
  };
  return { asked, followed, state, press };
}

test("a handler that prevents the link click gets no leave prompt and nothing navigates", () => {
  const { asked, followed, press } = setup();
  press(click("http://docflow.test/documents/57/history"), fakeEvent(), (event) => event.preventDefault());
  assert.deepEqual(asked, []);
  assert.deepEqual(followed, []);
});

test("an in-app link click is held so next/link does not navigate before the user answers", () => {
  const { press } = setup();
  const event = fakeEvent();
  let seenByLink: boolean | undefined;
  press(click("http://docflow.test/documents"), event, (e) => { seenByLink = e.defaultPrevented; });
  assert.equal(seenByLink, true);
});

test("confirming the prompt follows the link once", () => {
  const { asked, followed, press } = setup({ answer: true });
  press(click("http://docflow.test/documents"));
  assert.deepEqual(asked, [leaveMessage]);
  assert.deepEqual(followed, ["http://docflow.test/documents"]);
});

test("cancelling the prompt stays on the page", () => {
  const { asked, followed, press } = setup({ answer: false });
  press(click("http://docflow.test/documents"));
  assert.equal(asked.length, 1);
  assert.deepEqual(followed, []);
});

test("clicks are left alone when nothing is unsaved or the click does not leave the page", () => {
  const clean = setup({ dirty: false });
  const event = clean.press(click("http://docflow.test/documents"));
  assert.equal(event.defaultPrevented, false);
  const dirty = setup();
  for (const link of [click("http://docflow.test/documents", { target: "_blank" }), click("http://docflow.test/documents/57#roster"), click("http://docflow.test/documents", { defaultPrevented: true })]) {
    const before = link.defaultPrevented;
    assert.equal(dirty.press(link).defaultPrevented, before);
  }
  assert.deepEqual([...clean.asked, ...dirty.asked], []);
  assert.deepEqual([...clean.followed, ...dirty.followed], []);
});

test("a click whose handler clears the unsaved work follows the link without asking", () => {
  const { asked, followed, state, press } = setup();
  press(click("http://docflow.test/documents"), fakeEvent(), () => { state.dirty = false; });
  assert.deepEqual(asked, []);
  assert.deepEqual(followed, ["http://docflow.test/documents"]);
});

test("settling a click that was never held does nothing", () => {
  const { asked, followed, state } = setup();
  const guard = createLeaveGuard({ isDirty: () => state.dirty, currentHref: () => here, confirm: () => { asked.push("x"); return true; } });
  guard.settle(fakeEvent());
  assert.deepEqual(asked, []);
  assert.deepEqual(followed, []);
});

const field = (id: number, value: string, extra: Partial<ExtractedField> = {}): ExtractedField => ({ id, documentId: 57, fieldKey: "order_no", fieldLabel: "เลขที่คำสั่ง", fieldType: "text", value, sourceText: "", confidence: 0.9, pageNo: 1, boundingBox: "null", verified: false, ...extra });
const person = (id: number, extra: Partial<Appointment> = {}): Appointment => ({ id, documentId: 57, fullName: "สมชาย ใจดี", position: "อาจารย์", department: "ฝ่ายวิชาการ", committeeRole: "กรรมการ", responsibilities: "", confidence: 0.9, pageNo: 1, boundingBox: "null", verified: false, ...extra });
const saved = { fields: [field(1, "123/2569")], appointments: [person(10)], responsibilities: "ดูแลหลักสูตร", additionalReferences: "", rejectedGuesses: new Set<number>() };

test("the saved review is not a change", () => {
  assert.equal(reviewChanged(saved, structuredClone(saved)), false);
});

test("ui-only details of a row are not a change", () => {
  assert.equal(reviewChanged(saved, { ...saved, appointments: [person(10, { candidates: [], directoryUser: undefined })] }), false);
});

test("edits the reviewer would lose are changes", () => {
  assert.equal(reviewChanged(saved, { ...saved, fields: [field(1, "124/2569")] }), true);
  assert.equal(reviewChanged(saved, { ...saved, appointments: [person(10, { fullName: "สมชาย ใจดีมาก" })] }), true);
  assert.equal(reviewChanged(saved, { ...saved, appointments: [person(10, { directoryUserId: 4, nameMatchMethod: "MANUAL" })] }), true);
  assert.equal(reviewChanged(saved, { ...saved, appointments: [person(10, { extraDuties: "จัดตารางสอบ" })] }), true);
  assert.equal(reviewChanged(saved, { ...saved, appointments: [] }), true);
  assert.equal(reviewChanged(saved, { ...saved, responsibilities: "ดูแลหลักสูตรและการสอบ" }), true);
  assert.equal(reviewChanged(saved, { ...saved, additionalReferences: "ประกาศเพิ่มเติม" }), true);
});

// ชื่อใกล้เคียงที่ระบบเดาไว้ บันทึกแล้วจะผูกให้ ถ้าผู้ตรวจปฏิเสธ ค่าที่ส่งไปบันทึกก็เปลี่ยน
const guessed = person(10, { nameMatchMethod: "FUZZY", candidates: [{ userId: 7, fullName: "สมชาย ใจดี", positionTitle: "อาจารย์", score: 0.8 }] });

test("rejecting the system's guess is a change because the save no longer links that person", () => {
  const before = { ...saved, appointments: [guessed] };
  assert.equal(reviewChanged(before, { ...before, rejectedGuesses: new Set([10]) }), true);
});

test("a guess rejected before the last save is not a change", () => {
  const before = { ...saved, appointments: [guessed], rejectedGuesses: new Set([10]) };
  assert.equal(reviewChanged(before, { ...before, rejectedGuesses: new Set([10]) }), false);
});

test("rejecting a row that has no guess changes nothing that is saved", () => {
  const linked = { ...saved, appointments: [person(10, { directoryUserId: 4, nameMatchMethod: "EXACT" })] };
  assert.equal(reviewChanged(linked, { ...linked, rejectedGuesses: new Set([10]) }), false);
  assert.equal(reviewChanged(saved, { ...saved, rejectedGuesses: new Set([10]) }), false);
});

const duties = { extra: "", editing: false, draft: "", duty: "ดูแลสถานที่" };

test("an empty duties panel has nothing pending", () => {
  assert.equal(dutiesPending(duties), false);
  assert.equal(dutiesPending({ ...duties, extra: "   \n " }), false);
});

test("duty text typed but not yet added with เพิ่มหน้าที่ is pending", () => {
  assert.equal(dutiesPending({ ...duties, extra: "จัดทำรายงานสรุปผล" }), true);
});

test("an open duty edit is pending only when its text differs from the duty", () => {
  assert.equal(dutiesPending({ ...duties, editing: true, draft: "ดูแลสถานที่" }), false);
  assert.equal(dutiesPending({ ...duties, editing: true, draft: "ดูแลสถานที่และความปลอดภัย" }), true);
  assert.equal(dutiesPending({ ...duties, editing: false, draft: "ดูแลสถานที่และความปลอดภัย" }), false);
});
