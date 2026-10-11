import { computed, nextTick, reactive, shallowRef, useTemplateRef, watch } from "vue";
import { ArrowUp } from "@element-plus/icons-vue";
import type { InputInstance } from "element-plus";
import { PORTAL_DEFAULT_IMAGE_ASPECT_RATIO, PORTAL_IMAGE_ASPECT_RATIOS, PORTAL_IMAGE_RESOLUTIONS } from "./contract";
import { formatMultiplier } from "../utils";
import { resolveOpenAIImageSize } from "./openaiImageSizing";
import { usePortalImageReferences, type PortalImageReferenceMove } from "./usePortalImageReferences";
import type { PortalImageModelRecord, PortalImageTaskPayload } from "./types";

export interface PortalImageComposerProps {
  models: PortalImageModelRecord[];
  submitting: boolean;
  submitIcon?: unknown;
  notifyError?: (message: string) => void;
  submitTask: (payload: PortalImageTaskPayload, onCreated: () => void) => Promise<void>;
}

export function usePortalImageComposer(props: PortalImageComposerProps) {
  const models = computed(() => props.models);
  const submitting = computed(() => props.submitting);

  const {
    references: sourceImageReferences,
    mask: maskImageReference,
    addReferences,
    removeReference,
    moveReference,
    setMask,
    resetReferences
  } = usePortalImageReferences();

  const promptEditorExpanded = shallowRef(false);

  const maskImageInputKey = shallowRef(0);

  const composerComposing = shallowRef(false);

  const composerInputRef = useTemplateRef<InputInstance>("composerInputRef");

  const expandedPromptInputRef = useTemplateRef<InputInstance>("expandedPromptInputRef");

  const ui = reactive({
    operation: "generation" as "generation" | "edit"
  });

  const generationForm = reactive({
    model: "",
    prompt: "",
    n: 1,
    resolution: "auto",
    aspectRatio: PORTAL_DEFAULT_IMAGE_ASPECT_RATIO
  });

  const editForm = reactive({
    model: "",
    prompt: "",
    n: 1,
    resolution: "auto",
    aspectRatio: PORTAL_DEFAULT_IMAGE_ASPECT_RATIO
  });

  const resolutionOptions = PORTAL_IMAGE_RESOLUTIONS;

  const aspectRatioOptions = PORTAL_IMAGE_ASPECT_RATIOS;

  const selectedGenerationModel = computed(() => models.value.find((item) => modelOptionKey(item) === generationForm.model) || null);

  const selectedEditModel = computed(() => models.value.find((item) => modelOptionKey(item) === editForm.model) || null);

  const activeForm = computed(() => (ui.operation === "generation" ? generationForm : editForm));

  const activeOutputCountMax = computed(() => {
    const model = ui.operation === "generation" ? selectedGenerationModel.value : selectedEditModel.value;
    return ui.operation === "generation" ? model?.max_output_count || 1 : model?.edit_max_output_count || 1;
  });

  const submitIconComponent = computed(() => props.submitIcon || ArrowUp);

  const sourceImageFiles = computed(() => sourceImageReferences.value.map((item) => item.file));

  const maskImageFile = computed(() => maskImageReference.value?.file || null);

  const maskImageSummary = computed(() => (maskImageFile.value ? maskImageFile.value.name : "蒙版（可选）"));

  const operationOptions = computed(() => [
    { label: "文生图", value: "generation", disabled: sourceImageReferences.value.length > 0 },
    { label: "参考图", value: "edit" }
  ]);

  const canSubmit = computed(() => {
    if (submitting.value) return false;
    if (ui.operation === "generation") {
      return Boolean(selectedGenerationModel.value && generationForm.prompt.trim());
    }
    if (sourceImageFiles.value.length === 0) return false;
    return Boolean(selectedEditModel.value && editForm.prompt.trim());
  });

  const promptPlaceholder = computed(() => {
    return ui.operation === "edit" ? "描述你希望修改成什么样" : "描述你希望生成的画面";
  });

  function clampActiveOutputCount() {
    activeForm.value.n = Math.max(1, Math.min(activeOutputCountMax.value, activeForm.value.n || 1));
  }

  function modelOptionKey(model: PortalImageModelRecord) {
    return `${model.group_id || ""}::${model.model_code}`;
  }

  function modelGroupLabel(model: PortalImageModelRecord) {
    return model.billing_group_label || model.group_name || model.group_id || "默认分组";
  }

  function modelMultiplierLabel(model: PortalImageModelRecord) {
    return typeof model.effective_user_multiplier === "number" ? `${formatMultiplier(model.effective_user_multiplier)}x` : "";
  }

  function modelOptionMeta(model: PortalImageModelRecord) {
    const groupLabel = modelGroupLabel(model);
    const multiplierLabel = modelMultiplierLabel(model);
    return multiplierLabel && !groupLabel.includes(multiplierLabel) ? `${groupLabel} · ${multiplierLabel}` : groupLabel;
  }

  function modelOptionLabel(model: PortalImageModelRecord) {
    const suffix = modelOptionMeta(model);
    return suffix ? `${model.model_code} · ${suffix}` : model.model_code;
  }

  function handleAddReferences(files: File[]) {
    const result = addReferences(files);
    if (result.rejected > 0) props.notifyError?.("仅支持上传图片文件");
    if (result.added > 0) ui.operation = "edit";
  }

  function handleRemoveReference(id: string) {
    const removedIndex = removeReference(id);
    if (removedIndex === null) return;
    editForm.prompt = editForm.prompt.replace(/@图片(\d+)/g, (token, rawNumber: string) => {
      const number = Number(rawNumber);
      if (number === removedIndex + 1) return "";
      return number > removedIndex + 1 ? `@图片${number - 1}` : token;
    });
    if (sourceImageReferences.value.length === 0) clearMaskImage();
  }

  function handleMoveReference(payload: { id: string; direction: PortalImageReferenceMove }) {
    const moved = moveReference(payload.id, payload.direction);
    if (!moved) return;
    editForm.prompt = editForm.prompt.replace(/@图片(\d+)/g, (token, rawNumber: string) => {
      const number = Number(rawNumber);
      if (number === moved.from + 1) return `@图片${moved.to + 1}`;
      if (number === moved.to + 1) return `@图片${moved.from + 1}`;
      return token;
    });
  }

  function insertReferenceToken(index: number, input: InputInstance | null = composerInputRef.value) {
    const token = `@图片${index + 1}`;
    const textarea = input?.textarea;
    const prompt = editForm.prompt;
    const start = textarea?.selectionStart ?? prompt.length;
    const end = textarea?.selectionEnd ?? start;
    const before = prompt.slice(0, start);
    const after = prompt.slice(end);
    const prefix = before && !/\s$/.test(before) ? " " : "";
    const suffix = " ";
    const inserted = `${prefix}${token}${suffix}`;
    editForm.prompt = `${before}${inserted}${after}`;
    const cursor = before.length + inserted.length;
    void nextTick(() => {
      textarea?.focus();
      textarea?.setSelectionRange(cursor, cursor);
    });
  }

  function insertExpandedReferenceToken(index: number) {
    insertReferenceToken(index, expandedPromptInputRef.value);
  }

  function onMaskImageChange(event: Event) {
    const input = event.target as HTMLInputElement;
    setMask(input.files?.[0] || null);
    input.value = "";
  }

  function clearMaskImage() {
    setMask(null);
    maskImageInputKey.value += 1;
  }

  function handleComposerKeydown(event: KeyboardEvent) {
    if (event.isComposing || composerComposing.value || event.keyCode === 229) return;
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      void submit();
    }
  }

  function resetComposerInputs() {
    if (ui.operation === "generation") {
      generationForm.prompt = "";
      return;
    }
    editForm.prompt = "";
    resetReferences();
    maskImageInputKey.value += 1;
  }

  async function submitFromExpandedEditor() {
    if (!canSubmit.value) return;
    promptEditorExpanded.value = false;
    await submit();
  }

  function buildTaskPayload() {
    if (ui.operation === "generation") {
      const model = selectedGenerationModel.value;
      return {
        operation: "generation" as const,
        model: model?.model_code || "",
        group_id: model?.group_id,
        prompt: generationForm.prompt.trim(),
        n: generationForm.n,
        size: resolveOpenAIImageSize(generationForm.resolution, generationForm.aspectRatio)
      };
    }
    const model = selectedEditModel.value;
    const form = new FormData();
    form.append("operation", "edit");
    for (const file of sourceImageFiles.value) {
      form.append("image[]", file);
    }
    if (maskImageFile.value) form.append("mask", maskImageFile.value);
    form.append("model", model?.model_code || "");
    if (model?.group_id) form.append("group_id", model.group_id);
    form.append("prompt", editForm.prompt.trim());
    form.append("n", String(editForm.n));
    form.append("size", resolveOpenAIImageSize(editForm.resolution, editForm.aspectRatio));
    return form;
  }

  watch([selectedGenerationModel, selectedEditModel], () => clampActiveOutputCount());
  watch(() => ui.operation, () => {
    clampActiveOutputCount();
  });

  watch(
    () => ui.operation,
    (operation, previousOperation) => {
      if (operation === "edit" && previousOperation === "generation" && !editForm.prompt.trim()) {
        editForm.prompt = generationForm.prompt;
      }
    }
  );

  watch(models, (items) => {
    if (!generationForm.model && items.length) generationForm.model = modelOptionKey(items[0]);
    if (!editForm.model && items.length) editForm.model = modelOptionKey(items[0]);
  }, { immediate: true });

  async function submit() {
    if (!canSubmit.value) return;
    await props.submitTask(buildTaskPayload(), resetComposerInputs);
  }

  return {
    sourceImageReferences,
    handleAddReferences,
    handleRemoveReference,
    handleMoveReference,
    insertReferenceToken,
    activeForm,
    promptPlaceholder,
    handleComposerKeydown,
    composerComposing,
    promptEditorExpanded,
    ui,
    operationOptions,
    modelOptionKey,
    modelOptionLabel,
    modelOptionMeta,
    maskImageFile,
    activeOutputCountMax,
    resolutionOptions,
    aspectRatioOptions,
    maskImageInputKey,
    onMaskImageChange,
    maskImageSummary,
    clearMaskImage,
    canSubmit,
    submit,
    submitIconComponent,
    insertExpandedReferenceToken,
    submitFromExpandedEditor
  };
}
