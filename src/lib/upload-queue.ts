// เรียงตามชื่อแบบที่คนอ่าน: เลขในชื่อเทียบตามค่า "คำสั่ง-2" จึงมาก่อน "คำสั่ง-10"
const fileNameCollator = new Intl.Collator("th", { numeric: true, sensitivity: "base" });

export function sortByFileName<T extends { name: string }>(files: T[]): T[] {
  return [...files].sort((left, right) => fileNameCollator.compare(left.name, right.name));
}

export type UploadOutcome =
  | { kind: "saved" }
  | { kind: "duplicate"; duplicateDocumentId?: number; canOverwrite: boolean; message?: string }
  | { kind: "failed"; message: string };

type QueueOptions<T> = {
  send: (item: T, signal: AbortSignal) => Promise<UploadOutcome>;
  onDone: (item: T, outcome: UploadOutcome) => void;
  onStart?: (item: T) => void;
  onIdle?: () => void;
  // ไฟล์ใหญ่ใช้เวลาส่งนานกว่า จึงให้กำหนดเวลาตามไฟล์ได้
  timeoutMs?: number | ((item: T) => number);
};

const UPLOADS_AT_ONCE = 3;
const UPLOAD_TIMEOUT_MS = 2 * 60 * 1000;

// ไฟล์ที่ค้างเกินเวลาถูกยกเลิกและนับว่าไม่สำเร็จ ไม่ให้ไฟล์เดียวทำทั้งชุดค้าง
async function sendWithTimeout<T>(item: T, send: QueueOptions<T>["send"], timeoutMs: number): Promise<UploadOutcome> {
  const controller = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timedOut = new Promise<UploadOutcome>((resolve) => {
    timer = setTimeout(() => {
      controller.abort();
      resolve({ kind: "failed", message: "ส่งไฟล์นานเกินไป กดลองอีกครั้งได้ ถ้าไฟล์บันทึกไปแล้วระบบจะแจ้งว่าซ้ำ" });
    }, timeoutMs);
  });
  try {
    return await Promise.race([send(item, controller.signal), timedOut]);
  } finally {
    clearTimeout(timer);
  }
}

// คิวเดียวที่เติมไฟล์เพิ่มได้ตลอด ทุกชุดใช้ช่องส่งร่วมกัน จึงส่งพร้อมกันไม่เกิน 3 ไฟล์เสมอ
// ไฟล์ซ้ำถูกรายงานแล้วไปไฟล์ถัดไปทันที ผู้ใช้ค่อยเลือกบันทึกแทนจากแถวของไฟล์นั้นทีหลัง
export function createUploadQueue<T>({ send, onDone, onStart, onIdle, timeoutMs = UPLOAD_TIMEOUT_MS }: QueueOptions<T>) {
  const pending: T[] = [];
  let active = 0;
  const limitFor = (item: T) => typeof timeoutMs === "function" ? timeoutMs(item) : timeoutMs;
  const pump = () => {
    while (active < UPLOADS_AT_ONCE && pending.length > 0) {
      const item = pending.shift() as T;
      active += 1;
      onStart?.(item);
      void sendWithTimeout(item, send, limitFor(item)).then((outcome) => {
        active -= 1;
        onDone(item, outcome);
        if (active === 0 && pending.length === 0) onIdle?.();
        pump();
      });
    }
  };
  return { add: (items: T[]) => { pending.push(...items); pump(); } };
}

export function runUploadQueue<T>(items: T[], options: Omit<QueueOptions<T>, "onIdle">): Promise<void> {
  if (items.length === 0) return Promise.resolve();
  return new Promise((resolve) => createUploadQueue({ ...options, onIdle: resolve }).add(items));
}
