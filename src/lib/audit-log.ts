// Pure helpers for the audit log page. The API does the filtering; these
// functions only shape what the page sends and how a row reads.

export const AUDIT_PAGE_SIZE = 50;

export type AuditFilters = {
  q: string;
  action: string;
  userId: string;
  from: string;
  to: string;
};

export function emptyAuditFilters(): AuditFilters {
  return { q: "", action: "", userId: "", from: "", to: "" };
}

const bangkokTime = new Intl.DateTimeFormat("th-TH", {
  timeZone: "Asia/Bangkok",
  day: "numeric",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

// The list and the export share one filter query, so both show the same rows.
function filterParams(filters: AuditFilters) {
  const params = new URLSearchParams();
  const values: [string, string][] = [["q", filters.q], ["userId", filters.userId], ["action", filters.action], ["from", filters.from], ["to", filters.to]];
  for (const [key, value] of values) {
    const trimmed = value.trim();
    if (trimmed) params.set(key, trimmed);
  }
  return params;
}

export function auditQueryString(filters: AuditFilters, page?: number, pageSize?: number) {
  const params = filterParams(filters);
  if (page !== undefined) params.set("page", String(page));
  if (pageSize !== undefined) params.set("pageSize", String(pageSize));
  return params.toString();
}

export function auditExportPath(filters: AuditFilters) {
  const query = filterParams(filters).toString();
  return query ? `/api/v1/audit-logs/export.csv?${query}` : "/api/v1/audit-logs/export.csv";
}

// The API rejects a reversed range, so the page checks first and shows the message inline.
export function auditDateRangeError(from: string, to: string) {
  if (from && to && to < from) return "วันเริ่มต้นต้องไม่หลังวันสิ้นสุด";
  return "";
}

export function formatAuditTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return bangkokTime.format(date);
}

export function truncateDetails(text: string, limit = 120) {
  const characters = Array.from(text);
  if (characters.length <= limit) return { preview: text, truncated: false };
  return { preview: characters.slice(0, limit).join(""), truncated: true };
}

export function auditDocumentHref(item: { documentId?: number | null; documentExists: boolean }) {
  return item.documentExists && item.documentId ? `/documents/${item.documentId}` : null;
}
