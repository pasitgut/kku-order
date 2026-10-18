import { test } from "node:test";
import assert from "node:assert/strict";
import { filenameFromDisposition, reportQuery } from "./report.ts";

test("reportQuery sends only the filters that are set", () => {
  assert.equal(reportQuery("pdf", { person: 12, from: "2025-10-01", to: "2026-09-30", activeOnly: true }), "format=pdf&person=12&from=2025-10-01&to=2026-09-30&activeOnly=1");
  assert.equal(reportQuery("xlsx", {}), "format=xlsx");
  assert.equal(reportQuery("docx", { from: "2026-01-01", activeOnly: false }), "format=docx&from=2026-01-01");
  assert.equal(reportQuery("pdf", { name: "ชญาดา สุระวนิชกุล" }), `format=pdf&name=${new URLSearchParams({ n: "ชญาดา สุระวนิชกุล" }).toString().slice(2)}`);
});

test("filenameFromDisposition prefers the UTF-8 Thai name", () => {
  const header = `attachment; filename="docflow-report.pdf"; filename*=UTF-8''${encodeURIComponent("รายงานคำสั่งแต่งตั้ง-นางสาวจิราภรณ์ หอมอ่อน.pdf")}`;
  assert.equal(filenameFromDisposition(header, "x.pdf"), "รายงานคำสั่งแต่งตั้ง-นางสาวจิราภรณ์ หอมอ่อน.pdf");
  assert.equal(filenameFromDisposition(`attachment; filename="docflow-report.xlsx"`, "x.xlsx"), "docflow-report.xlsx");
  assert.equal(filenameFromDisposition(null, "x.docx"), "x.docx");
});
