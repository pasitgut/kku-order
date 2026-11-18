export type Panel<T> = { data: T; error: "" } | { data: null; error: string };

type Loaders = Record<string, () => Promise<unknown>>;

// กลุ่ม "รอยืนยัน" ที่ backend กรองด้วยตัวกรองเดียวกับตัวเลขในภาพรวม เอกสารที่ OCR ล้มเหลวจึงไม่โผล่ในรายการ
export const awaitingConfirmationQuery = { status: "AWAITING_CONFIRMATION", limit: 5 };

// โหลดแต่ละส่วนของหน้าแยกกัน ส่วนที่ล้มเหลวแสดงข้อผิดพลาดของตัวเอง ส่วนอื่นยังแสดงข้อมูลได้
export async function loadPanels<L extends Loaders>(loaders: L): Promise<{ [K in keyof L]: Panel<Awaited<ReturnType<L[K]>>> }> {
  const keys = Object.keys(loaders) as (keyof L)[];
  const settled = await Promise.allSettled(keys.map((key) => loaders[key]()));
  const panels = {} as { [K in keyof L]: Panel<Awaited<ReturnType<L[K]>>> };
  keys.forEach((key, index) => {
    const result = settled[index];
    panels[key] = (result.status === "fulfilled"
      ? { data: result.value, error: "" }
      : { data: null, error: result.reason instanceof Error ? result.reason.message : "โหลดข้อมูลไม่สำเร็จ" }) as Panel<Awaited<ReturnType<L[typeof key]>>>;
  });
  return panels;
}
