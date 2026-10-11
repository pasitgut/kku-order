import { test } from "node:test";
import assert from "node:assert/strict";
import { AUDIT_PAGE_SIZE, auditDateRangeError, auditDocumentHref, auditExportPath, auditQueryString, emptyAuditFilters, formatAuditTime, truncateDetails } from "./audit-log.ts";

test("empty filters produce no query parameters", () => {
  assert.equal(auditQueryString(emptyAuditFilters()), "");
  assert.equal(auditQueryString(emptyAuditFilters(), 2, AUDIT_PAGE_SIZE), "page=2&pageSize=50");
});

test("filters are trimmed and only non-empty values are sent", () => {
  const query = auditQueryString({ ...emptyAuditFilters(), q: "  ตรวจ  ", userId: " kit ", action: "DOCUMENT_UPDATED", from: "2026-10-01", to: "" }, 1, 50);
  assert.equal(query, "q=%E0%B8%95%E0%B8%A3%E0%B8%A7%E0%B8%88&userId=kit&action=DOCUMENT_UPDATED&from=2026-10-01&page=1&pageSize=50");
});

test("the export path carries the same filters without paging", () => {
  const path = auditExportPath({ ...emptyAuditFilters(), action: "DOCUMENT_CONFIRMED" });
  assert.equal(path, "/api/v1/audit-logs/export.csv?action=DOCUMENT_CONFIRMED");
  assert.ok(!path.includes("page"));
});

test("a range that ends before it starts is reported in Thai", () => {
  assert.equal(auditDateRangeError("2026-10-11", "2026-10-10"), "วันเริ่มต้นต้องไม่หลังวันสิ้นสุด");
  assert.equal(auditDateRangeError("2026-10-10", "2026-10-10"), "");
  assert.equal(auditDateRangeError("", "2026-10-10"), "");
});

test("times are shown in Bangkok time with Thai month names", () => {
  // 20:05 UTC on 10 October is 03:05 on 11 October in Bangkok.
  assert.equal(formatAuditTime("2026-10-10T20:05:00Z"), "11 ต.ค. 2569 03:05");
  assert.equal(formatAuditTime("not a date"), "not a date");
});

test("long details are cut on a character boundary and flagged", () => {
  const thai = "ก".repeat(130);
  const short = truncateDetails("สั้น");
  assert.deepEqual(short, { preview: "สั้น", truncated: false });
  const long = truncateDetails(thai, 120);
  assert.equal(long.truncated, true);
  assert.equal(Array.from(long.preview).length, 120);
});

test("a document link exists only while the document does", () => {
  assert.equal(auditDocumentHref({ documentId: 7, documentExists: true }), "/documents/7");
  assert.equal(auditDocumentHref({ documentId: null, documentExists: false }), null);
  assert.equal(auditDocumentHref({ documentId: 7, documentExists: false }), null);
});
