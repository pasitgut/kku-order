import { test } from "node:test";
import assert from "node:assert/strict";
import { createUploadQueue, runUploadQueue, sortByFileName, type UploadOutcome } from "./upload-queue.ts";

test("files sort by name with numbers compared by value, Thai names before Latin", () => {
  const names = ["คำสั่ง-10.pdf", "คำสั่ง-2.pdf", "รายชื่อ.xlsx", "Annex.pdf", "คำสั่ง-1.pdf"];
  const sorted = sortByFileName(names.map((name) => ({ name }))).map((file) => file.name);
  assert.deepEqual(sorted, ["คำสั่ง-1.pdf", "คำสั่ง-2.pdf", "คำสั่ง-10.pdf", "รายชื่อ.xlsx", "Annex.pdf"]);
});

test("a duplicate file is reported and the next file still uploads", async () => {
  const replies: Record<string, UploadOutcome> = {
    "a.pdf": { kind: "duplicate", duplicateDocumentId: 57, canOverwrite: true },
    "b.pdf": { kind: "saved" },
  };
  const finished: [string, UploadOutcome][] = [];
  await runUploadQueue([{ name: "a.pdf" }, { name: "b.pdf" }], {
    send: async (file) => replies[file.name],
    onDone: (file, outcome) => finished.push([file.name, outcome]),
  });
  assert.deepEqual(finished, [
    ["a.pdf", { kind: "duplicate", duplicateDocumentId: 57, canOverwrite: true }],
    ["b.pdf", { kind: "saved" }],
  ]);
});

test("uploads three files at a time, never more", async () => {
  let inFlight = 0;
  let mostAtOnce = 0;
  const files = ["1", "2", "3", "4", "5", "6", "7"].map((name) => ({ name }));
  await runUploadQueue(files, {
    send: async () => {
      inFlight += 1;
      mostAtOnce = Math.max(mostAtOnce, inFlight);
      await new Promise((resolve) => setTimeout(resolve, 5));
      inFlight -= 1;
      return { kind: "saved" };
    },
    onDone: () => {},
  });
  assert.equal(mostAtOnce, 3);
});

test("a file that takes too long is marked failed and cancelled, and the queue moves on", async () => {
  const finished: [string, UploadOutcome["kind"]][] = [];
  let stuckSignal: AbortSignal | undefined;
  await runUploadQueue([{ name: "stuck.pdf" }, { name: "ok.pdf" }], {
    timeoutMs: 20,
    send: (file, signal) => {
      if (file.name === "ok.pdf") return Promise.resolve({ kind: "saved" });
      stuckSignal = signal;
      return new Promise(() => {});
    },
    onDone: (file, outcome) => finished.push([file.name, outcome.kind]),
  });
  assert.deepEqual(finished, [["ok.pdf", "saved"], ["stuck.pdf", "failed"]]);
  assert.equal(stuckSignal?.aborted, true);
});

test("a file is reported as started only when it actually begins sending", async () => {
  const started: string[] = [];
  const release: (() => void)[] = [];
  const files = ["1", "2", "3", "4"].map((name) => ({ name }));
  const done = runUploadQueue(files, {
    onStart: (file) => started.push(file.name),
    send: () => new Promise((resolve) => release.push(() => resolve({ kind: "saved" }))),
    onDone: () => {},
  });
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(started, ["1", "2", "3"]);
  release.shift()?.();
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(started, ["1", "2", "3", "4"]);
  release.forEach((finish) => finish());
  await done;
});

test("files added while others are uploading share the same three slots", async () => {
  let inFlight = 0;
  let mostAtOnce = 0;
  const finished: string[] = [];
  const idle = new Promise<void>((resolve) => {
    const queue = createUploadQueue<{ name: string }>({
      send: async () => {
        inFlight += 1;
        mostAtOnce = Math.max(mostAtOnce, inFlight);
        await new Promise((wait) => setTimeout(wait, 5));
        inFlight -= 1;
        return { kind: "saved" };
      },
      onDone: (file) => finished.push(file.name),
      onIdle: resolve,
    });
    queue.add([{ name: "a" }, { name: "b" }, { name: "c" }]);
    queue.add([{ name: "d" }, { name: "e" }]);
  });
  await idle;
  assert.equal(mostAtOnce, 3);
  assert.deepEqual([...finished].sort(), ["a", "b", "c", "d", "e"]);
});

test("the time limit can grow with the file", async () => {
  const finished: [string, UploadOutcome["kind"]][] = [];
  await runUploadQueue([{ name: "small", ms: 20 }, { name: "large", ms: 60 }], {
    timeoutMs: (file) => file.ms,
    send: () => new Promise((resolve) => setTimeout(() => resolve({ kind: "saved" }), 40)),
    onDone: (file, outcome) => finished.push([file.name, outcome.kind]),
  });
  assert.deepEqual(finished, [["small", "failed"], ["large", "saved"]]);
});
