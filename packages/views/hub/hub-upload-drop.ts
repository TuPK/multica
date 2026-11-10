import {
  isSkillArchiveFilename,
  prepareSkillArchiveFromPickerFiles,
  wrapExistingSkillArchive,
  type PreparedSkillArchive,
} from "@multica/core/skills/pack-archive";

/**
 * Turns a drop into the File list that packages/core/skills/pack-archive.ts
 * expects. Dropped folders arrive as FileSystemEntry trees with no
 * webkitRelativePath, so each file gets one set from its tree position.
 */

export async function filesFromDrop(
  data: Pick<DataTransfer, "items" | "files">,
): Promise<File[]> {
  const entries: FileSystemEntry[] = [];
  for (const item of Array.from(data.items ?? [])) {
    if (item.kind !== "file" || typeof item.webkitGetAsEntry !== "function") continue;
    const entry = item.webkitGetAsEntry();
    if (entry) entries.push(entry);
  }
  if (entries.some((entry) => entry.isDirectory)) return filesFromEntries(entries);
  return Array.from(data.files ?? []);
}

export async function filesFromEntries(entries: readonly FileSystemEntry[]): Promise<File[]> {
  const out: File[] = [];
  for (const entry of entries) await collect(entry, "", out);
  return out;
}

export function withRelativePath(file: File, path: string): File {
  Object.defineProperty(file, "webkitRelativePath", { value: path, configurable: true });
  return file;
}

async function collect(entry: FileSystemEntry, parent: string, out: File[]): Promise<void> {
  const path = `${parent}${entry.name}`;
  if (entry.isFile) {
    const file = await readFile(entry as FileSystemFileEntry);
    out.push(withRelativePath(file, path));
    return;
  }
  for (const child of await readDirectory(entry as FileSystemDirectoryEntry)) {
    await collect(child, `${path}/`, out);
  }
}

function readFile(entry: FileSystemFileEntry): Promise<File> {
  return new Promise((resolve, reject) => entry.file(resolve, reject));
}

// readEntries returns at most one batch per call; an empty batch ends the directory.
async function readDirectory(dir: FileSystemDirectoryEntry): Promise<FileSystemEntry[]> {
  const reader = dir.createReader();
  const all: FileSystemEntry[] = [];
  for (;;) {
    const batch = await new Promise<FileSystemEntry[]>((resolve, reject) =>
      reader.readEntries(resolve, reject),
    );
    if (batch.length === 0) return all;
    all.push(...batch);
  }
}

/**
 * One archive file is used as-is; anything else is a folder selection, packed
 * by the shared skill import. A single file with a relative path came from a
 * folder, so it is not treated as an archive.
 */
export function prepareImportFiles(files: File[]): Promise<PreparedSkillArchive> {
  const [only] = files;
  if (files.length === 1 && only && !only.webkitRelativePath && isSkillArchiveFilename(only.name)) {
    return Promise.resolve(wrapExistingSkillArchive(only));
  }
  return prepareSkillArchiveFromPickerFiles(files);
}
