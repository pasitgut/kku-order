"use client";

import { useState } from "react";
import {
  Archive,
  ArrowDown,
  ArrowRight,
  ArrowsClockwise,
  BellRinging,
  Check,
  CheckCircle,
  CloudArrowDown,
  Database,
  FileArrowUp,
  FileText,
  GearSix,
  IdentificationCard,
  Key,
  MagnifyingGlass,
  PersonSimple,
  ShieldCheck,
  Stack,
  UsersThree,
  WarningCircle,
  Wrench,
} from "@phosphor-icons/react";

type DiagramKey = "context" | "workflow" | "data" | "roles" | "lifecycle";

const tabs: { key: DiagramKey; label: string; caption: string }[] = [
  { key: "context", label: "ภาพรวมระบบ", caption: "ใครเชื่อมกับอะไร" },
  { key: "workflow", label: "ขั้นตอนทำงาน", caption: "ตั้งแต่รับไฟล์จนยืนยัน" },
  { key: "data", label: "การไหลของข้อมูล", caption: "ข้อมูลถูกเก็บและส่งต่อ" },
  { key: "roles", label: "บทบาทผู้ใช้งาน", caption: "ใครทำอะไรได้บ้าง" },
  { key: "lifecycle", label: "วงจรเอกสาร", caption: "ติดตามสถานะตลอดอายุ" },
];

function Node({ icon: Icon, eyebrow, title, detail, tone = "blue" }: { icon: typeof FileText; eyebrow?: string; title: string; detail?: string; tone?: "blue" | "navy" | "green" | "gold" | "gray" }) {
  return <div className={`diagram-node diagram-node-${tone}`}>
    <span className="diagram-node-icon"><Icon size={21} weight="duotone" /></span>
    <span className="min-w-0"><span className="diagram-node-eyebrow">{eyebrow}</span><strong>{title}</strong>{detail && <small>{detail}</small>}</span>
  </div>;
}

function Arrow({ label, vertical = false }: { label?: string; vertical?: boolean }) {
  return <div className={`diagram-arrow ${vertical ? "diagram-arrow-vertical" : ""}`} aria-hidden="true"><span>{label}</span>{vertical ? <ArrowDown size={19} weight="bold" /> : <ArrowRight size={19} weight="bold" />}</div>;
}

function DiagramHeader({ number, title, description }: { number: string; title: string; description: string }) {
  return <div className="diagram-heading"><span className="diagram-number">{number}</span><div><h2>{title}</h2><p>{description}</p></div></div>;
}

function ContextDiagram() {
  return <div className="diagram-surface">
    <DiagramHeader number="01" title="System context — ระบบอยู่ตรงกลางของงานจริง" description="มองเห็นผู้เกี่ยวข้อง แหล่งข้อมูล และผลลัพธ์ในภาพเดียว เหมาะสำหรับอธิบายขอบเขตของโปรเจคกับอาจารย์" />
    <div className="context-map">
      <div className="context-column">
        <p className="diagram-label">ผู้ใช้งานและระบบภายนอก</p>
        <Node icon={UsersThree} eyebrow="People" title="เจ้าหน้าที่คณะ" detail="นำเข้า · ตรวจสอบ · ยืนยัน" tone="navy" />
        <Node icon={PersonSimple} eyebrow="People" title="ผู้ดูข้อมูล" detail="ค้นหาเอกสารและประวัติ" tone="gray" />
        <Node icon={CloudArrowDown} eyebrow="External source" title="Google Drive" detail="โฟลเดอร์เอกสารกลางของคณะ" tone="blue" />
        <Node icon={IdentificationCard} eyebrow="External source" title="Directory API" detail="ข้อมูลบุคลากรสำหรับจับคู่ชื่อ" tone="blue" />
      </div>

      <div className="context-core">
        <p className="diagram-label">ระบบที่นำเสนอ</p>
        <div className="context-core-box">
          <div className="context-core-title"><Stack size={26} weight="duotone" /><span><strong>DocFlow</strong><small>ศูนย์กลางเอกสารคำสั่งแต่งตั้ง</small></span></div>
          <div className="context-core-grid"><span><FileText size={17} /> จัดเก็บเอกสาร</span><span><MagnifyingGlass size={17} /> ค้นหาและรายงาน</span><span><CheckCircle size={17} /> ตรวจสอบก่อนยืนยัน</span><span><ArrowsClockwise size={17} /> นำเข้าอัตโนมัติ</span></div>
        </div>
        <div className="context-connector-row"><Arrow label="เข้าใช้งาน" /><Arrow label="ส่งข้อมูล" /></div>
      </div>

      <div className="context-column">
        <p className="diagram-label">บริการภายในระบบ</p>
        <Node icon={Wrench} eyebrow="Processing" title="one-ocr + Parser" detail="สกัดข้อความ ชื่อ ตำแหน่ง และวันที่" tone="gold" />
        <Node icon={Database} eyebrow="Data store" title="MySQL" detail="metadata · OCR · revision · audit" tone="green" />
        <Node icon={Archive} eyebrow="File store" title="Document storage" detail="เก็บไฟล์ต้นฉบับที่นำเข้า" tone="green" />
        <Node icon={BellRinging} eyebrow="Notification" title="อีเมลแจ้งเตือน" detail="ล่วงหน้า 3, 2 และ 1 เดือน" tone="gold" />
      </div>
    </div>
    <div className="diagram-callout"><ShieldCheck size={19} weight="duotone" /><span><strong>หลักคิดสำคัญ:</strong> ผู้ใช้ไม่ต้องรู้รายละเอียด OCR หรือฐานข้อมูล — เห็นเฉพาะคิวงาน สถานะ และจุดที่ต้องตัดสินใจ</span></div>
  </div>;
}

const workflowSteps = [
  { icon: GearSix, index: "01", title: "ตั้งค่าแหล่งนำเข้า", detail: "กำหนดโฟลเดอร์ Google Drive และรอบการตรวจ", tone: "blue" },
  { icon: FileArrowUp, index: "02", title: "รับไฟล์เอกสาร", detail: "อัปโหลดเองหรือระบบดึงไฟล์ใหม่/ไฟล์ที่แก้ไข", tone: "navy" },
  { icon: Wrench, index: "03", title: "สกัดข้อมูล", detail: "PDF → render → one-ocr หรืออ่าน structured file", tone: "gold" },
  { icon: MagnifyingGlass, index: "04", title: "เจ้าหน้าที่ตรวจสอบ", detail: "แก้ผลสกัดและผูกชื่อกับบุคลากรในระบบ", tone: "gold" },
  { icon: CheckCircle, index: "05", title: "ยืนยันเอกสาร", detail: "เปลี่ยนเป็นข้อมูลอ้างอิงที่ค้นหาได้", tone: "green" },
  { icon: BellRinging, index: "06", title: "ติดตามต่อเนื่อง", detail: "ค้นย้อนหลังและแจ้งเตือนก่อนหมดวาระ", tone: "blue" },
];

function WorkflowDiagram() {
  return <div className="diagram-surface">
    <DiagramHeader number="02" title="Operational workflow — งานจริงของเจ้าหน้าที่" description="ลำดับงานที่เกิดขึ้นจริง ตั้งแต่เอกสารเข้าระบบจนกลายเป็นฐานข้อมูลที่ค้นหาและติดตามได้" />
    <div className="workflow-track">{workflowSteps.map((step, index) => { const Icon = step.icon; return <div className="workflow-item" key={step.index}>
      <div className={`workflow-node workflow-node-${step.tone}`}><span className="workflow-index">{step.index}</span><Icon size={25} weight="duotone" /></div>
      <div className="workflow-copy"><strong>{step.title}</strong><span>{step.detail}</span></div>
      {index < workflowSteps.length - 1 && <Arrow />}
    </div>; })}</div>
    <div className="workflow-gates"><div><span className="gate-dot gate-blue" /><strong>อัตโนมัติ</strong><small>sync / OCR / แจ้งเตือน</small></div><div><span className="gate-dot gate-gold" /><strong>ต้องใช้ดุลยพินิจ</strong><small>ตรวจชื่อ ตำแหน่ง และยืนยัน</small></div><div><span className="gate-dot gate-green" /><strong>ตรวจสอบย้อนหลัง</strong><small>revision และ audit log</small></div></div>
    <div className="diagram-callout"><Check size={19} weight="bold" /><span><strong>จุดที่ระบบช่วยคณะได้จริง:</strong> ลดการคีย์ข้อมูลซ้ำ แต่ยังคงให้เจ้าหน้าที่เป็นผู้ตัดสินใจก่อนข้อมูลจะถูกยืนยัน</span></div>
  </div>;
}

function DataDiagram() {
  return <div className="diagram-surface">
    <DiagramHeader number="03" title="Data flow — ข้อมูลเดินทางอย่างไร" description="แยกให้เห็นชัดว่าอะไรคือ input, อะไรคือการประมวลผล และอะไรคือข้อมูลอ้างอิงที่คณะนำไปใช้ต่อ" />
    <div className="data-flow-grid">
      <div className="data-flow-column"><p className="diagram-label">INPUT</p><Node icon={FileArrowUp} title="PDF / DOCX / XLSX / CSV" detail="ไฟล์ต้นฉบับจากเจ้าหน้าที่หรือ Google Drive" tone="blue" /><Node icon={IdentificationCard} title="Directory API" detail="ชื่อ อีเมล ตำแหน่ง และหน่วยงาน" tone="blue" /><Node icon={Key} title="SSO / Identity" detail="ยืนยันตัวตนและบทบาทผู้ใช้" tone="navy" /></div>
      <div className="data-flow-column data-flow-middle"><p className="diagram-label">PROCESSING</p><div className="process-stack"><div><Wrench size={20} /><span><strong>Validation</strong><small>ชนิดไฟล์และความปลอดภัย</small></span></div><Arrow vertical label="ไฟล์ที่ผ่านการตรวจ" /><div><Stack size={20} /><span><strong>OCR / Structured extraction</strong><small>ข้อความ บรรทัด ตำแหน่ง และความมั่นใจ</small></span></div><Arrow vertical label="ข้อมูลรอตรวจ" /><div><CheckCircle size={20} /><span><strong>Human review</strong><small>แก้ไข ผูกบุคคล และยืนยัน</small></span></div></div></div>
      <div className="data-flow-column"><p className="diagram-label">OUTPUT & DATA STORE</p><Node icon={Database} title="MySQL" detail="metadata · extracted fields · OCR · audit" tone="green" /><Node icon={Archive} title="Document storage" detail="ไฟล์ต้นฉบับสำหรับเปิดดูย้อนหลัง" tone="green" /><Node icon={MagnifyingGlass} title="Search / Report" detail="ค้นหาชื่อ เลขที่คำสั่ง ตำแหน่ง และ OCR" tone="navy" /><Node icon={BellRinging} title="Expiry notifications" detail="แจ้งเตือนผู้เกี่ยวข้องก่อนหมดวาระ" tone="gold" /></div>
    </div>
    <div className="data-contract"><span className="data-contract-key">ข้อมูลหลักที่ต้องรักษา</span><span>ไฟล์ต้นฉบับ</span><Arrow /><span>ผลสกัด</span><Arrow /><span>การตัดสินใจของเจ้าหน้าที่</span><Arrow /><span>ประวัติที่ตรวจสอบได้</span></div>
  </div>;
}

const roleRows = [
  { role: "ผู้ดูแลระบบ", badge: "ADMIN", icon: ShieldCheck, tone: "navy", actions: ["ตั้งค่า OCR / Google Drive", "จัดการผู้ใช้งานและสิทธิ์", "ลบเอกสารตามนโยบาย"] },
  { role: "เจ้าหน้าที่", badge: "STAFF", icon: UsersThree, tone: "blue", actions: ["นำเข้าและแทนที่เอกสาร", "ตรวจผลสกัดและผูกบุคคล", "ยืนยันเอกสาร / sync บุคลากร"] },
  { role: "ผู้ดูข้อมูล", badge: "VIEWER", icon: MagnifyingGlass, tone: "gray", actions: ["ค้นหาเอกสารและประวัติ", "เปิดดูไฟล์และข้อมูลสกัด", "ดูรายงานที่ได้รับอนุญาต"] },
  { role: "นักพัฒนา", badge: "DEVELOPER", icon: Wrench, tone: "gold", actions: ["ดูข้อมูลเพื่อดูแลระบบ", "ลบเอกสารเมื่อได้รับสิทธิ์", "ไม่ใช่ผู้ยืนยันข้อมูลประจำวัน"] },
];

function RolesDiagram() {
  return <div className="diagram-surface">
    <DiagramHeader number="04" title="Role & responsibility — แบ่งงานตามความรับผิดชอบ" description="แสดงขอบเขตการใช้งานให้ชัดว่าใครเป็นผู้ดำเนินการ ใครเป็นผู้ตัดสินใจ และใครเป็นผู้ดูข้อมูล" />
    <div className="roles-board">{roleRows.map((row) => { const Icon = row.icon; return <div className="role-row" key={row.badge}><div className={`role-identity role-${row.tone}`}><span><Icon size={22} weight="duotone" /></span><div><strong>{row.role}</strong><small>{row.badge}</small></div></div><div className="role-actions">{row.actions.map((action) => <span key={action}><Check size={14} weight="bold" />{action}</span>)}</div></div>; })}</div>
    <div className="role-principle"><span className="principle-mark"><ShieldCheck size={22} weight="duotone" /></span><div><strong>หลักการออกแบบสิทธิ์</strong><p>อ่านได้ตามบทบาท แต่การเปลี่ยนแปลงข้อมูลสำคัญต้องผ่าน ADMIN หรือ STAFF และการยืนยันยังคงมีชื่อผู้ดำเนินการกับเวลาเก็บไว้ในระบบ</p></div></div>
  </div>;
}

const lifecycle = [
  { state: "นำเข้า", label: "INGESTED", detail: "บันทึกไฟล์และ metadata", tone: "blue" },
  { state: "กำลังประมวลผล", label: "PROCESSING", detail: "OCR / parser ทำงานเบื้องหลัง", tone: "blue" },
  { state: "รอตรวจสอบ", label: "REVIEW", detail: "เจ้าหน้าที่ตรวจผลสกัด", tone: "gold" },
  { state: "ยืนยันแล้ว", label: "CONFIRMED", detail: "ใช้เป็นข้อมูลอ้างอิงได้", tone: "green" },
  { state: "ติดตามหมดวาระ", label: "EXPIRY WATCH", detail: "แจ้งเตือน 3 / 2 / 1 เดือน", tone: "navy" },
];

function LifecycleDiagram() {
  return <div className="diagram-surface">
    <DiagramHeader number="05" title="Document lifecycle — เอกสารหนึ่งฉบับมีสถานะอะไรบ้าง" description="ช่วยตอบคำถามสำคัญในการใช้งานจริง: ตอนนี้เอกสารอยู่ขั้นไหน ใครต้องทำอะไรต่อ และเมื่อไรจึงถือว่าเชื่อถือได้" />
    <div className="lifecycle-track">{lifecycle.map((item, index) => <div className="lifecycle-item" key={item.label}><div className={`lifecycle-marker lifecycle-${item.tone}`}><span>{index + 1}</span></div><div><strong>{item.state}</strong><small>{item.label}</small><p>{item.detail}</p></div>{index < lifecycle.length - 1 && <Arrow />}</div>)}</div>
    <div className="lifecycle-notes"><div><span className="note-icon note-gold"><WarningCircle size={18} weight="duotone" /></span><span><strong>ถ้าข้อมูลไม่ชัด</strong><small>ย้อนกลับไปแก้ไขผลสกัดหรือผูกชื่อใหม่ได้</small></span></div><div><span className="note-icon note-green"><CheckCircle size={18} weight="duotone" /></span><span><strong>เมื่อยืนยันแล้ว</strong><small>ยังแก้ไขได้ตามสิทธิ์ และมี revision รองรับ</small></span></div><div><span className="note-icon note-blue"><BellRinging size={18} weight="duotone" /></span><span><strong>หลังยืนยัน</strong><small>ระบบติดตามวันหมดวาระให้ต่อเนื่อง</small></span></div></div>
  </div>;
}

function ActiveDiagram({ active }: { active: DiagramKey }) {
  if (active === "workflow") return <WorkflowDiagram />;
  if (active === "data") return <DataDiagram />;
  if (active === "roles") return <RolesDiagram />;
  if (active === "lifecycle") return <LifecycleDiagram />;
  return <ContextDiagram />;
}

export function SystemDiagrams() {
  const [active, setActive] = useState<DiagramKey>("context");
  const activeTab = tabs.find((tab) => tab.key === active) ?? tabs[0];

  return <div className="space-y-6">
    <section className="presentation-intro"><div><p className="presentation-kicker">PRESENTATION KIT · DOCFLOW</p><h2>แผนภาพที่ช่วยเล่าโปรเจคให้เห็นภาพการใช้งานจริง</h2><p>ใช้หน้านี้เป็นสคริปต์ประกอบการนำเสนอ: เริ่มจากปัญหาและขอบเขตระบบ แล้วค่อยไล่ workflow, data flow, สิทธิ์ และวงจรเอกสาร</p></div><div className="presentation-actions"><button type="button" onClick={() => window.print()} className="focus-ring print-button"><FileText size={17} />พิมพ์ / บันทึกเป็น PDF</button><span>แนะนำให้เปิดแบบเต็มหน้าจอเมื่อสาธิต</span></div></section>
    <section className="presentation-outcomes"><div><span>01</span><strong>ลดงานคีย์ซ้ำ</strong><small>นำเข้าและสกัดข้อมูลให้อัตโนมัติ</small></div><div><span>02</span><strong>ตรวจสอบได้</strong><small>มีคนรับผิดชอบ สถานะ และ revision</small></div><div><span>03</span><strong>ใช้ต่อเนื่อง</strong><small>ค้นย้อนหลังและติดตามหมดวาระ</small></div></section>
    <section className="diagram-tabs" aria-label="มุมมองแผนภาพ"><div className="diagram-tabs-label"><span>เลือกมุมมอง</span><strong>{activeTab.label}</strong></div><div className="diagram-tab-list">{tabs.map((tab) => <button key={tab.key} type="button" onClick={() => setActive(tab.key)} className={`focus-ring diagram-tab ${active === tab.key ? "diagram-tab-active" : ""}`}><span>{tab.label}</span><small>{tab.caption}</small></button>)}</div></section>
    <ActiveDiagram active={active} />
  </div>;
}
