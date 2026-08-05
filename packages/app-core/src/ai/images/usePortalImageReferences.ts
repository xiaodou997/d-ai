import { onScopeDispose, readonly, shallowRef } from "vue";

export interface PortalImageReference {
  id: string;
  file: File;
  previewUrl: string;
}

export type PortalImageReferenceMove = -1 | 1;

export function usePortalImageReferences() {
  const references = shallowRef<PortalImageReference[]>([]);
  const mask = shallowRef<PortalImageReference | null>(null);
  let nextReferenceId = 0;

  function createReference(file: File): PortalImageReference {
    nextReferenceId += 1;
    return {
      id: `reference-${nextReferenceId}`,
      file,
      previewUrl: URL.createObjectURL(file)
    };
  }

  function addReferences(files: File[]) {
    const accepted = files.filter((file) => !file.type || file.type.startsWith("image/"));
    const rejected = files.filter((file) => file.type && !file.type.startsWith("image/"));
    if (accepted.length > 0) {
      references.value = [...references.value, ...accepted.map(createReference)];
    }
    return { added: accepted.length, rejected: rejected.length };
  }

  function removeReference(id: string): number | null {
    const index = references.value.findIndex((item) => item.id === id);
    if (index < 0) return null;
    URL.revokeObjectURL(references.value[index].previewUrl);
    references.value = references.value.filter((item) => item.id !== id);
    return index;
  }

  function moveReference(id: string, direction: PortalImageReferenceMove): { from: number; to: number } | null {
    const from = references.value.findIndex((item) => item.id === id);
    const to = from + direction;
    if (from < 0 || to < 0 || to >= references.value.length) return null;
    const next = [...references.value];
    [next[from], next[to]] = [next[to], next[from]];
    references.value = next;
    return { from, to };
  }

  function setMask(file: File | null) {
    if (mask.value) URL.revokeObjectURL(mask.value.previewUrl);
    mask.value = file ? createReference(file) : null;
  }

  function resetReferences() {
    for (const item of references.value) URL.revokeObjectURL(item.previewUrl);
    if (mask.value) URL.revokeObjectURL(mask.value.previewUrl);
    references.value = [];
    mask.value = null;
  }

  onScopeDispose(resetReferences);

  return {
    references: readonly(references),
    mask: readonly(mask),
    addReferences,
    removeReference,
    moveReference,
    setMask,
    resetReferences
  };
}
