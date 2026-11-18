import Link from "next/link";
import { FileX } from "@phosphor-icons/react/dist/ssr";
import { AppShell } from "@/components/app-shell";

export default function NotFound() {
  return <AppShell breadcrumb="หน้าหลัก / ไม่พบหน้า" title="ไม่พบหน้าที่ต้องการ" description="ลิงก์อาจพิมพ์ผิด หรือหน้านี้ถูกย้ายหรือลบไปแล้ว">
    <section className="card flex flex-col items-center gap-3 px-6 py-14 text-center">
      <span className="flex h-12 w-12 items-center justify-center rounded-lg bg-[var(--accent-soft)] text-[var(--accent)]"><FileX size={24} weight="duotone" /></span>
      <h2 className="text-lg font-semibold text-[var(--navy)]">ไม่พบหน้านี้ในระบบ (404)</h2>
      <p className="max-w-md text-sm font-light leading-6 text-[var(--muted)]">ตรวจที่อยู่อีกครั้ง หรือกลับไปเริ่มจากหน้าหลักหรือรายการเอกสาร</p>
      <div className="mt-2 flex flex-wrap justify-center gap-2">
        <Link href="/" className="focus-ring inline-flex h-11 items-center rounded-md bg-[var(--accent)] px-5 text-sm font-medium text-white hover:bg-[var(--accent-strong)]">กลับหน้าหลัก</Link>
        <Link href="/documents" className="focus-ring inline-flex h-11 items-center rounded-md border border-[var(--accent)] px-5 text-sm font-medium text-[var(--accent)] hover:bg-[var(--accent-soft)]">ดูเอกสารทั้งหมด</Link>
      </div>
    </section>
  </AppShell>;
}
