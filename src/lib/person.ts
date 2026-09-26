const titlePattern = /^(?:(?:นางสาว|นาง|นาย|ผู้ช่วยศาสตราจารย์|รองศาสตราจารย์|ศาสตราจารย์|ผศ\.|รศ\.|ศ\.|ดร\.|อ\.|Mr\.|Mrs\.|Ms\.|Dr\.)\s*)+/i;

// สระหน้าไม่ใช่ตัวแรกของชื่อที่คนอ่าน เช่น "เอกชัย" ควรได้ "อ"
const leadingVowels = /^[เแโใไ]+/;

function firstLetter(word: string) {
  return word.replace(leadingVowels, "").charAt(0);
}

export function initials(name: string) {
  const words = name.trim().replace(titlePattern, "").trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return "?";
  if (words.length === 1) return words[0].replace(leadingVowels, "").slice(0, 2).toUpperCase();
  return (firstLetter(words[0]) + firstLetter(words[words.length - 1])).toUpperCase();
}
