import { test } from "node:test";
import assert from "node:assert/strict";
import { canReprocess, confidenceLabel, isInsecureURL, processingNote } from "./processing-status.ts";

const base = { status: "PROCESSING", confidence: 0 };

test("RETRY_WAIT shows the upcoming attempt number and Bangkok time", () => {
  const note = processingNote({
    ...base,
    processingStage: "RETRY_WAIT",
    extractionAttempts: 1,
    nextAttemptAt: "2026-10-11T02:30:00Z",
  });
  assert.equal(note, "รอลองใหม่ ครั้งที่ 2 เวลา 09:30");
});

test("FAILED shows failure note", () => {
  assert.equal(processingNote({ status: "NEEDS_REVIEW", processingStage: "FAILED", confidence: 0 }), "ประมวลผลไม่สำเร็จ");
});

test("normal documents have no note", () => {
  assert.equal(processingNote({ status: "REVIEW", processingStage: "EXTRACTED", confidence: 0.9 }), null);
});

test("reprocess allowed only for FAILED or RETRY_WAIT", () => {
  assert.equal(canReprocess({ status: "NEEDS_REVIEW", processingStage: "FAILED", confidence: 0 }), true);
  assert.equal(canReprocess({ status: "PROCESSING", processingStage: "RETRY_WAIT", confidence: 0 }), true);
  assert.equal(canReprocess({ status: "REVIEW", processingStage: "EXTRACTED", confidence: 0.9 }), false);
  assert.equal(canReprocess({ status: "PROCESSING", processingStage: "EXTRACTING", confidence: 0 }), false);
});

test("confidence 0 is not shown as calculating", () => {
  assert.equal(confidenceLabel({ status: "NEEDS_REVIEW", processingStage: "EXTRACTED", confidence: 0 }), "ไม่ระบุ");
});

test("processing documents show calculating label", () => {
  assert.equal(confidenceLabel({ status: "PROCESSING", processingStage: "EXTRACTING", confidence: 0 }), "กำลังคำนวณ");
});

test("known confidence is shown as percent (confidence is 0-1)", () => {
  assert.equal(confidenceLabel({ status: "REVIEW", processingStage: "EXTRACTED", confidence: 0.87 }), "87%");
});

test("http url is flagged as insecure", () => {
  assert.equal(isInsecureURL("http://10.0.0.5:8000"), true);
  assert.equal(isInsecureURL("https://ocr.example"), false);
  assert.equal(isInsecureURL(""), false);
});
