// ตรงกับ requireRole ใน backend/main.go ถ้าแก้ฝั่ง backend ต้องแก้ที่นี่ด้วย
export const ROLES = ["ADMIN", "STAFF", "DEVELOPER", "VIEWER"] as const;
export type Role = (typeof ROLES)[number];

export type PermissionKey = "view" | "import" | "edit" | "confirm" | "createPerson" | "sync" | "delete" | "settings";

export const PERMISSIONS: { key: PermissionKey; label: string; roles: Role[] }[] = [
  { key: "view", label: "ดูเอกสาร ค้นหา และรายงาน", roles: ["ADMIN", "STAFF", "DEVELOPER", "VIEWER"] },
  { key: "import", label: "นำเข้าเอกสารและอัปโหลดไฟล์ทับ", roles: ["ADMIN", "STAFF"] },
  { key: "edit", label: "แก้ไขผลสกัดและผูกรายชื่อ", roles: ["ADMIN", "STAFF"] },
  { key: "confirm", label: "ยืนยันเอกสาร", roles: ["ADMIN", "STAFF"] },
  { key: "createPerson", label: "เพิ่มบุคคลใหม่ในฐานข้อมูล", roles: ["ADMIN", "STAFF"] },
  { key: "sync", label: "sync บุคลากรจาก External API", roles: ["ADMIN", "STAFF"] },
  { key: "delete", label: "ลบเอกสาร", roles: ["ADMIN", "DEVELOPER"] },
  { key: "settings", label: "แก้ไขการตั้งค่า OCR และการนำเข้า", roles: ["ADMIN"] },
];

const roleLabels: Record<Role, string> = { ADMIN: "ผู้ดูแลระบบ", STAFF: "เจ้าหน้าที่", DEVELOPER: "นักพัฒนา", VIEWER: "ผู้ดูข้อมูล" };

function normalizeRole(role: string): Role | null {
  const upper = role.trim().toUpperCase();
  return (ROLES as readonly string[]).includes(upper) ? (upper as Role) : null;
}

export function can(role: string, key: PermissionKey) {
  const normalized = normalizeRole(role);
  if (!normalized) return false;
  return PERMISSIONS.some((permission) => permission.key === key && permission.roles.includes(normalized));
}

export function roleLabel(role: string) {
  const normalized = normalizeRole(role);
  return normalized ? roleLabels[normalized] : role;
}
