import { AppShell } from "@/components/app-shell";
import { SystemDiagrams } from "@/components/system-diagrams";

export default function DiagramsPage() {
  return <AppShell breadcrumb="เครื่องมือสำหรับการนำเสนอ / แผนภาพระบบ" title="แผนภาพระบบ DocFlow" description="ชุดมุมมองสำหรับอธิบายการใช้งานจริงของระบบจัดการเอกสารคำสั่งแต่งตั้ง">
    <SystemDiagrams />
  </AppShell>;
}
