import { test } from "node:test";
import assert from "node:assert/strict";
import { canRescan, confidenceLabel, isInsecureURL, processingNote, rescanNeedsConfirm } from "./processing-status.ts";

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

test("rescan allowed for finished, failed and retry-waiting documents", () => {
  assert.equal(canRescan({ status: "NEEDS_REVIEW", processingStage: "FAILED", confidence: 0 }), true);
  assert.equal(canRescan({ status: "PROCESSING", processingStage: "RETRY_WAIT", confidence: 0 }), true);
  assert.equal(canRescan({ status: "REVIEW", processingStage: "EXTRACTED", confidence: 0.9 }), true);
  assert.equal(canRescan({ status: "CONFIRMED", processingStage: "EXTRACTED", confidence: 0.95 }), true);
});

test("rescan refused while queued, extracting or preparing", () => {
  assert.equal(canRescan({ status: "PROCESSING", processingStage: "QUEUED", confidence: 0 }), false);
  assert.equal(canRescan({ status: "PROCESSING", processingStage: "EXTRACTING", confidence: 0 }), false);
  assert.equal(canRescan({ status: "PROCESSING", processingStage: "PREPARING", confidence: 0 }), false);
});

test("rescan refused for cancelled documents whatever the stage", () => {
  assert.equal(canRescan({ status: "CANCELLED", processingStage: "EXTRACTED", confidence: 0.9 }), false);
  assert.equal(canRescan({ status: "CANCELLED", processingStage: "FAILED", confidence: 0 }), false);
});

test("only a finished document needs the two-step confirmation", () => {
  assert.equal(rescanNeedsConfirm({ status: "NEEDS_REVIEW", processingStage: "FAILED", confidence: 0 }), false);
  assert.equal(rescanNeedsConfirm({ status: "PROCESSING", processingStage: "RETRY_WAIT", confidence: 0 }), false);
  assert.equal(rescanNeedsConfirm({ status: "REVIEW", processingStage: "EXTRACTED", confidence: 0.9 }), true);
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
