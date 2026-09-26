// แปลง cron ที่ใช้บ่อยใน .env ให้อ่านง่าย ถ้าไม่รู้จักแสดงค่าเดิม
export function cronLabel(expression: string) {
  const parts = expression.trim().split(/\s+/);
  if (parts.length !== 5) return expression;
  const [minute, hour, day, month, weekday] = parts;
  const everyDay = day === "*" && month === "*" && weekday === "*";
  if (everyDay && /^\d+$/.test(minute) && /^\d+$/.test(hour)) {
    return `ทุกวัน ${hour.padStart(2, "0")}:${minute.padStart(2, "0")} น.`;
  }
  if (everyDay && hour === "*") {
    if (minute === "*") return "ทุก 1 นาที";
    const step = minute.match(/^\*\/(\d+)$/);
    if (step) return `ทุก ${step[1]} นาที`;
  }
  return expression;
}
