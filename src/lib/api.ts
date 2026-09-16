// The default is a same-origin Next.js proxy. In Docker this keeps browser
// requests on the web origin while Next forwards them to the `api` service.
// Set NEXT_PUBLIC_API_URL only when the API is intentionally exposed directly.
const API_URL = (process.env.NEXT_PUBLIC_API_URL ?? "/backend").replace(/\/$/, "");

export type ExtractedField = {
  id: number;
  documentId: number;
  fieldKey: string;
  fieldLabel: string;
  fieldType: string;
  value: string;
  sourceText: string;
  confidence: number;
  pageNo: number;
  boundingBox: string;
  verified: boolean;
  verifiedBy?: string;
  verifiedAt?: string;
};

export type Appointment = {
  id: number;
  documentId: number;
  fullName: string;
  position: string;
  department: string;
  committeeRole: string;
  responsibilities: string;
  directoryUserId?: number;
  directoryUser?: DirectoryUser;
  nameMatchMethod?: string;
  nameMatchScore?: number;
  confidence: number;
  pageNo: number;
  boundingBox: string;
  verified: boolean;
};

export type OCRLine = {
  id: number;
  pageNo: number;
  lineIndex: number;
  text: string;
  confidence: number;
  boundingBox: string;
};

export type OCRPage = {
  id: number;
  pageNo: number;
  imageWidth: number;
  imageHeight: number;
  imageAngle: number;
  rawText: string;
  status: string;
  lines: OCRLine[];
};

export type ApiDocument = {
  id: number;
  title: string;
  originalFilename: string;
  sourceType: string;
  mimeType: string;
  orderType: string;
  orderYearBE: number;
  status: string;
  orderNo: string;
  committee: string;
  signerName: string;
  signerPosition: string;
  responsibilities: string;
  additionalReferences: string;
  confidentiality: string;
  qualityProfile: string;
  pageCount: number;
  personCount: number;
  fileSizeBytes: number;
  retentionUntil?: string;
  importSource: string;
  processingStage: string;
  processingStartedAt?: string;
  processingFinishedAt?: string;
  processingDurationMs: number;
  reviewNotes: string;
  issuedDate?: string;
  effectiveDate?: string;
  expiryDate?: string;
  confidence: number;
  importedBy: string;
  confirmedBy: string;
  confirmedAt?: string;
  ocrProvider: string;
  ocrText: string;
  ocrError: string;
  fields: ExtractedField[];
  appointments: Appointment[];
  ocrPages: OCRPage[];
  revisions?: DocumentRevision[];
  createdAt: string;
  updatedAt: string;
};

export type DocumentRevision = {
  id: number;
  documentId: number;
  fieldKey: string;
  oldValue: string;
  newValue: string;
  changedBy: string;
  changedAt: string;
};

export type Dashboard = {
  total: number;
  processing: number;
  review: number;
  confirmed: number;
  needsReview: number;
  expiring: number;
};

export type DirectoryUser = {
  id: number;
  userId: string;
  externalId: string;
  prefix: string;
  username: string;
  fullName: string;
  firstName: string;
  lastName: string;
  gender: string;
  email: string;
  phone: string;
  phoneFormatted: string;
  department: string;
  faculty: string;
  positionTitle: string;
  jobTitle: string;
  positionEn: string;
  positionPrefixEn: string;
  managePosition: string;
  nameEn: string;
  suffixEn: string;
  scopusId: string;
  scholarAuthorId: string;
  labName: string;
  room: string;
  cpWebId: string;
  roleId: number;
  roleName: string;
  role: string;
  isActive: string;
  sourceUpdatedAt: string;
  lastSeenAt?: string;
  syncedAt?: string;
};

export type SyncRun = { id: number; startedAt: string; completedAt?: string; status: string; fetched: number; created: number; updated: number; unchanged: number; errorMessage?: string };
export type ExpiryNotification = { id: number; documentId: number; appointmentId?: number; noticeType: string; dueAt: string; status: string };
export type WorkloadReportRow = { fullName: string; position: string; documentCount: number; appointmentCount: number; verifiedCount: number };

type ListResponse = { data: ApiDocument[]; pagination?: { total: number; page: number; limit: number } };

export class ApiError extends Error {
  constructor(message: string, public readonly status: number, public readonly details: Record<string, unknown> = {}) {
    super(message);
    this.name = "ApiError";
  }
}

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_URL}${path}`, { ...init, credentials: "include", cache: "no-store" });
  if (!response.ok) {
    let message = "ไม่สามารถเชื่อมต่อระบบได้";
    let details: Record<string, unknown> = {};
    try {
      details = await response.json() as Record<string, unknown>;
      message = typeof details.error === "string" ? details.error : message;
    } catch {
      // Keep the Thai fallback when the server does not return JSON.
    }
    throw new ApiError(message, response.status, details);
  }
  return response.json() as Promise<T>;
}

export function getApiUrl() {
  return API_URL;
}

export function getDocumentPageImageUrl(id: string | number, page: number) {
  return `${API_URL}/api/v1/documents/${id}/pages/${page}/image`;
}

export async function getDashboard() {
  return apiFetch<{ data: Dashboard }>("/api/v1/dashboard");
}

export async function getDocuments(params: { q?: string; status?: string; page?: number; limit?: number } = {}) {
  const query = new URLSearchParams();
  if (params.q) query.set("q", params.q);
  if (params.status && params.status !== "ALL") query.set("status", params.status);
  if (params.page) query.set("page", String(params.page));
  if (params.limit) query.set("limit", String(params.limit));
  return apiFetch<ListResponse>(`/api/v1/documents${query.size ? `?${query.toString()}` : ""}`);
}

export async function searchDocuments(params: { query?: string; position?: string; orderType?: string; dateFrom?: string; dateTo?: string } = {}) {
  const query = new URLSearchParams({ limit: "100" });
  if (params.query) query.set("q", params.query);
  if (params.position) query.set("position", params.position);
  if (params.orderType) query.set("orderType", params.orderType);
  if (params.dateFrom) query.set("dateFrom", params.dateFrom);
  if (params.dateTo) query.set("dateTo", params.dateTo);
  return apiFetch<ListResponse>(`/api/v1/search?${query.toString()}`);
}

export async function getWorkloadReport(params: { query?: string; dateFrom?: string; dateTo?: string } = {}) {
  const query = new URLSearchParams();
  if (params.query) query.set("q", params.query);
  if (params.dateFrom) query.set("dateFrom", params.dateFrom);
  if (params.dateTo) query.set("dateTo", params.dateTo);
  return apiFetch<{ data: WorkloadReportRow[] }>(`/api/v1/reports/workload${query.size ? `?${query.toString()}` : ""}`);
}

export async function getDocument(id: string | number) {
  return apiFetch<{ data: ApiDocument }>(`/api/v1/documents/${id}`);
}

export async function getDocumentRevisions(id: string | number) {
  return apiFetch<{ data: DocumentRevision[] }>(`/api/v1/documents/${id}/revisions`);
}

export async function getDirectoryUsers(query = "") {
  return apiFetch<{ data: DirectoryUser[] }>(`/api/v1/directory-users${query ? `?q=${encodeURIComponent(query)}` : ""}`);
}

export async function syncDirectoryUsers() {
  return apiFetch<{ data: { fetched: number; created: number; updated: number; unchanged: number } }>("/api/v1/sync/users", { method: "POST" });
}

export async function getSyncRuns() {
  return apiFetch<{ data: SyncRun[] }>("/api/v1/sync/runs");
}

export async function getExpiryNotifications() {
  return apiFetch<{ data: ExpiryNotification[] }>("/api/v1/notifications/expiry");
}

export async function uploadDocument(file: File, overwrite = false) {
  const body = new FormData();
  body.append("file", file);
  body.append("overwrite", String(overwrite));
  return apiFetch<{ data: ApiDocument }>("/api/v1/documents", {
    method: "POST",
    body,
  });
}

export async function updateDocument(id: number, payload: Record<string, unknown>) {
  return apiFetch<{ data: ApiDocument }>(`/api/v1/documents/${id}`, {
    method: "PATCH",
    body: JSON.stringify(payload),
    headers: { "Content-Type": "application/json" },
  });
}

export async function deleteDocument(id: number) {
  return apiFetch<{ message: string; documentId: number; storageCleanupError?: string }>(`/api/v1/documents/${id}`, {
    method: "DELETE",
  });
}

export async function confirmDocument(id: number) {
  return apiFetch<{ data: ApiDocument; message: string }>(`/api/v1/documents/${id}/confirm`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({}),
  });
}

export function formatThaiDate(value?: string) {
  if (!value) return "ไม่ระบุ";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value.slice(0, 10);
  return new Intl.DateTimeFormat("th-TH", { dateStyle: "medium" }).format(date);
}

export function toDateInput(value?: string) {
  return value ? value.slice(0, 10) : "";
}

export function formatFileSize(bytes: number) {
  if (!bytes) return "-";
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function statusLabel(status: string) {
  const labels: Record<string, string> = {
    PROCESSING: "กำลังประมวลผล",
    REVIEW: "รอตรวจสอบ",
    NEEDS_REVIEW: "ต้องตรวจสอบ",
    CONFIRMED: "ยืนยันแล้ว",
    FAILED: "ประมวลผลไม่สำเร็จ",
  };
  return labels[status] ?? status;
}
